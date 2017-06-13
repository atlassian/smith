package app

import (
	"context"
	"log"

	"github.com/atlassian/smith"
	"github.com/atlassian/smith/pkg/resources"

	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
)

// Name2Bundle is a function that does a lookup of Bundle based on its namespace and name.
type Name2Bundle func(namespace, bundleName string) (*smith.Bundle, error)

// resourceEventHandler handles events for objects with various kinds.
type resourceEventHandler struct {
	ctx         context.Context
	processor   Processor
	name2bundle Name2Bundle
}

func (h *resourceEventHandler) OnAdd(obj interface{}) {
	bundleName, namespace := getBundleNameAndNamespace(obj)
	h.rebuildByName(namespace, bundleName, "added", obj)
}

func (h *resourceEventHandler) OnUpdate(oldObj, newObj interface{}) {
	oldBundleName, oldNamespace := getBundleNameAndNamespace(oldObj)

	newBundleName, newNamespace := getBundleNameAndNamespace(newObj)

	if oldBundleName != newBundleName { // changed controller of the object
		h.rebuildByName(oldNamespace, oldBundleName, "updated", oldObj)
	}
	h.rebuildByName(newNamespace, newBundleName, "updated", newObj)
}

func (h *resourceEventHandler) OnDelete(obj interface{}) {
	meta, ok := obj.(meta_v1.Object)
	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			log.Printf("[REH] Delete event with unrecognized object type: %T", obj)
			return
		}
		meta, ok = tombstone.Obj.(meta_v1.Object)
		if !ok {
			log.Printf("[REH] Delete tombstone with unrecognized object type: %T", tombstone.Obj)
			return
		}
	}
	bundleName, namespace := getBundleNameAndNamespace(meta)
	h.rebuildByName(namespace, bundleName, "deleted", obj)
}

func (h *resourceEventHandler) rebuildByName(namespace, bundleName, addUpdateDelete string, obj interface{}) {
	if len(bundleName) == 0 {
		return
	}
	bundle, err := h.name2bundle(namespace, bundleName)
	if err != nil {
		log.Printf("[REH][%s/%s] Failed to do bundle lookup: %v", namespace, bundleName, err)
		return
	}
	if bundle != nil {
		log.Printf("[REH][%s/%s] Rebuilding bundle because resource %s was %s",
			namespace, bundleName, obj.(meta_v1.Object).GetName(), addUpdateDelete)
		if err = h.processor.Rebuild(h.ctx, bundle); err != nil && err != context.Canceled && err != context.DeadlineExceeded {
			log.Printf("[REH][%s/%s] Error rebuilding bundle: %v", namespace, bundleName, err)
		}
		//} else {
		// TODO bundle not found - handle deletion?
		// There may be a race between TPR instance informer and bundle informer in case of
		// connection loss. Because of that bundle informer might have stale cache without
		// a bundle for which TPR/resource informers already receive events. Because of that
		// bundle may be deleted erroneously (as it was not found in cache).
		// Need to handle this situation properly.
	}
}

func getBundleNameAndNamespace(obj interface{}) (string, string) {
	var bundleName string
	meta := obj.(meta_v1.Object)
	ref := resources.GetControllerOf(meta)
	if ref != nil && ref.APIVersion == smith.BundleResourceGroupVersion && ref.Kind == smith.BundleResourceKind {
		bundleName = ref.Name
	}
	return bundleName, meta.GetNamespace()
}
