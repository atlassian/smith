package bundlec

import (
	"context"

	"github.com/atlassian/ctrl"
	"github.com/atlassian/smith"
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/smith/pkg/resources"
	"github.com/atlassian/smith/pkg/util/logz"
	"go.uber.org/zap"
	apiext_v1b1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
)

type watchState struct {
	cancel context.CancelFunc
}

// crdEventHandler handles events for objects with Kind: CustomResourceDefinition.
// For each object a new informer is started to watch for events.
type crdEventHandler struct {
	*Controller
	watchers map[string]watchState // CRD name -> state
}

// OnAdd handles just added CRDs and CRDs that existed before CRD informer was started.
// Any CRDs that are not established and/or haven't had their names accepted are ignored.
// This is necessary to wait until a CRD has been processed by the CRD controller. Also see OnUpdate.
func (h *crdEventHandler) OnAdd(obj interface{}) {
	crd := obj.(*apiext_v1b1.CustomResourceDefinition)
	logger := h.loggerForObj(crd)
	if !supportEnabled(crd) {
		logger.Sugar().Debugf("Not setting up watch for CRD because %s annotation is not set to 'true'", smith.CrdSupportEnabled)
		return
	}
	if h.ensureWatch(logger, crd) {
		h.rebuildBundles(logger, crd, "added")
	}
}

// OnUpdate handles updates for CRDs.
// If
// - there is no watch and
// - a CRD is established and
// - it had its names accepted
// then a watch is established. This is necessary to wait until a CRD has been processed by the CRD controller and
// to pick up fixes for invalid/conflicting CRDs.
func (h *crdEventHandler) OnUpdate(oldObj, newObj interface{}) {
	newCrd := newObj.(*apiext_v1b1.CustomResourceDefinition)
	logger := h.loggerForObj(newCrd)
	if !supportEnabled(newCrd) {
		h.ensureNoWatch(logger, newCrd)
		return
	}
	if h.ensureWatch(logger, newCrd) {
		h.rebuildBundles(logger, newCrd, "updated")
	}
}

func (h *crdEventHandler) OnDelete(obj interface{}) {
	logger := h.loggerForObj(obj)
	crd, ok := obj.(*apiext_v1b1.CustomResourceDefinition)
	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			logger.Sugar().Errorf("Delete event with unrecognized object type: %T", obj)
			return
		}
		crd, ok = tombstone.Obj.(*apiext_v1b1.CustomResourceDefinition)
		if !ok {
			logger.Sugar().Errorf("Delete tombstone with unrecognized object type: %T", tombstone.Obj)
			return
		}
		logger = h.loggerForObj(crd)
	}
	if h.ensureNoWatch(logger, crd) {
		// Rebuild only if the watch was removed. Otherwise it is pointless.
		h.rebuildBundles(logger, crd, "deleted")
	}
}

// ensureWatch ensures there is a watch for CRs of a CRD.
// Returns true if a watch was found or set up successfully and false if there is no watch and it was not set up for
// some reason.
func (h *crdEventHandler) ensureWatch(logger *zap.Logger, crd *apiext_v1b1.CustomResourceDefinition) bool {
	if crd.Name == smith_v1.BundleResourceName {
		return false
	}
	if _, ok := h.watchers[crd.Name]; ok {
		return true
	}
	if !resources.IsCrdConditionTrue(crd, apiext_v1b1.Established) {
		logger.Info("Not adding a watch for CRD because it hasn't been established")
		return false
	}
	if !resources.IsCrdConditionTrue(crd, apiext_v1b1.NamesAccepted) {
		logger.Info("Not adding a watch for CRD because its names haven't been accepted")
		return false
	}
	gvk := schema.GroupVersionKind{
		Group:   crd.Spec.Group,
		Version: crd.Spec.Version,
		Kind:    crd.Spec.Names.Kind,
	}
	logger.Info("Configuring watch for CRD")
	res, err := h.SmartClient.ForGVK(gvk, h.Namespace)
	if err != nil {
		logger.Error("Failed to get client for CRD", zap.Error(err))
		return false
	}
	crdInf := cache.NewSharedIndexInformer(&cache.ListWatch{
		ListFunc: func(options meta_v1.ListOptions) (runtime.Object, error) {
			return res.List(options)
		},
		WatchFunc: func(options meta_v1.ListOptions) (watch.Interface, error) {
			return res.Watch(options)
		},
	}, &unstructured.Unstructured{}, h.CrdResyncPeriod, cache.Indexers{})
	h.wgLock.Lock()
	defer h.wgLock.Unlock()
	if h.stopping {
		return false
	}
	crdInf.AddEventHandler(h.resourceHandler)
	err = h.Store.AddInformer(gvk, crdInf)
	if err != nil {
		logger.Error("Failed to add informer for CRD to multisore", zap.Error(err))
		return false
	}
	ctx, cancel := context.WithCancel(h.crdContext)
	h.watchers[crd.Name] = watchState{cancel: cancel}
	h.wg.StartWithChannel(ctx.Done(), crdInf.Run)
	return true
}

// ensureNoWatch ensures there is no watch for CRs of a CRD.
// Returns true if a watch was found and terminated and false if there was no watch already.
func (h *crdEventHandler) ensureNoWatch(logger *zap.Logger, crd *apiext_v1b1.CustomResourceDefinition) bool {
	crdWatch, ok := h.watchers[crd.Name]
	if !ok {
		// Nothing to do. This can happen if there was an error adding a watch
		return false
	}
	logger.Info("Removing watch for CRD")
	crdWatch.cancel()
	delete(h.watchers, crd.Name)
	gvk := schema.GroupVersionKind{
		Group:   crd.Spec.Group,
		Version: crd.Spec.Version,
		Kind:    crd.Spec.Names.Kind,
	}
	h.Store.RemoveInformer(gvk)
	return true
}

func (h *crdEventHandler) rebuildBundles(logger *zap.Logger, crd *apiext_v1b1.CustomResourceDefinition, addUpdateDelete string) {
	bundles, err := h.BundleStore.GetBundlesByCrd(crd)
	if err != nil {
		logger.Error("Failed to get bundles by CRD name", zap.Error(err))
		return
	}
	for _, bundle := range bundles {
		logger.
			With(logz.Namespace(bundle), logz.Bundle(bundle)).
			Sugar().Infof("Rebuilding bundle because CRD was %s", addUpdateDelete)
		h.WorkQueue.Add(ctrl.QueueKey{
			Namespace: bundle.Namespace,
			Name:      bundle.Name,
		})
	}
}

func supportEnabled(crd *apiext_v1b1.CustomResourceDefinition) bool {
	return crd.Annotations[smith.CrdSupportEnabled] == "true"
}
