package controller

import (
	"context"
	"log"

	"github.com/atlassian/smith"
	"github.com/atlassian/smith/pkg/resources"

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
	ctx context.Context
	*BundleController
	watchers map[string]watchState // CRD name -> state
}

// OnAdd handles just added CRDs and CRDs that existed before CRD informer was started.
// Any CRDs that are not established and/or haven't had their names accepted are ignored.
// This is necessary to wait until a CRD has been processed by the CRD controller. Also see OnUpdate.
func (h *crdEventHandler) OnAdd(obj interface{}) {
	crd := obj.(*apiext_v1b1.CustomResourceDefinition)
	if h.ensureWatch(crd) {
		h.rebuildBundles(crd, "added")
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
	if h.ensureWatch(newCrd) {
		h.rebuildBundles(newCrd, "updated")
	}
}

func (h *crdEventHandler) OnDelete(obj interface{}) {
	crd, ok := obj.(*apiext_v1b1.CustomResourceDefinition)
	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			log.Printf("[CRDEH] Delete event with unrecognized object type: %T", obj)
			return
		}
		crd, ok = tombstone.Obj.(*apiext_v1b1.CustomResourceDefinition)
		if !ok {
			log.Printf("[CRDEH] Delete tombstone with unrecognized object type: %T", tombstone.Obj)
			return
		}
	}
	h.unwatch(crd)
	h.rebuildBundles(crd, "deleted")
}

func (h *crdEventHandler) ensureWatch(crd *apiext_v1b1.CustomResourceDefinition) bool {
	if crd.Name == smith.BundleResourceName {
		return false
	}
	if _, ok := h.watchers[crd.Name]; ok {
		return true
	}
	if !resources.IsCrdConditionTrue(crd, apiext_v1b1.Established) {
		log.Printf("[CRDEH] Not adding a watch for CRD %s because it hasn't been established", crd.Name)
		return false
	}
	if !resources.IsCrdConditionTrue(crd, apiext_v1b1.NamesAccepted) {
		log.Printf("[CRDEH] Not adding a watch for CRD %s because its names haven't been accepted", crd.Name)
		return false
	}
	gvk := schema.GroupVersionKind{
		Group:   crd.Spec.Group,
		Version: crd.Spec.Version,
		Kind:    crd.Spec.Names.Kind,
	}
	log.Printf("[CRDEH] Configuring watch for CRD %s", crd.Name)
	res, err := h.smartClient.ForGVK(gvk, meta_v1.NamespaceNone)
	if err != nil {
		log.Printf("[CRDEH] Failed to setup informer for CRD %s: %v", crd.Name, err)
		return false
	}
	crdInf := cache.NewSharedIndexInformer(&cache.ListWatch{
		ListFunc: func(options meta_v1.ListOptions) (runtime.Object, error) {
			return res.List(options)
		},
		WatchFunc: func(options meta_v1.ListOptions) (watch.Interface, error) {
			return res.Watch(options)
		},
	}, &unstructured.Unstructured{}, h.crdResyncPeriod, cache.Indexers{})
	crdInf.AddEventHandler(h.resourceHandler)
	ctx, cancel := context.WithCancel(h.ctx)
	h.watchers[crd.Name] = watchState{cancel: cancel}
	h.store.AddInformer(gvk, crdInf)
	h.wg.StartWithChannel(ctx.Done(), crdInf.Run)
	return true
}

func (h *crdEventHandler) unwatch(crd *apiext_v1b1.CustomResourceDefinition) {
	crdWatch, ok := h.watchers[crd.Name]
	if !ok {
		// Nothing to do. This can happen if there was an error adding a watch
		return
	}
	log.Printf("[CRDEH] Removing watch for CRD %s", crd.Name)
	crdWatch.cancel()
	delete(h.watchers, crd.Name)
	gvk := schema.GroupVersionKind{
		Group:   crd.Spec.Group,
		Version: crd.Spec.Version,
		Kind:    crd.Spec.Names.Kind,
	}
	h.store.RemoveInformer(gvk)
}

func (h *crdEventHandler) rebuildBundles(crd *apiext_v1b1.CustomResourceDefinition, addUpdateDelete string) {
	bundles, err := h.bundleStore.GetBundlesByCrd(crd)
	if err != nil {
		log.Printf("[CRDEH] Failed to get bundles by CRD name %s: %v", crd.Name, err)
		return
	}
	for _, bundle := range bundles {
		log.Printf("[CRDEH][%s/%s] Rebuilding bundle because CRD %s was %s", bundle.Namespace, bundle.Name, crd.Name, addUpdateDelete)
		h.enqueue(bundle)
	}
}
