package app

import (
	"context"
	"log"

	"github.com/atlassian/smith"
	"github.com/atlassian/smith/pkg/resources"

	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"
)

type bundleStoreInterface interface {
	// Get is a function that does a lookup of Bundle based on its namespace and name.
	Get(namespace, bundleName string) (*smith.Bundle, error)
	GetBundlesByObject(gk schema.GroupKind, namespace, name string) ([]*smith.Bundle, error)
}

// resourceEventHandler handles events for objects with various kinds.
type resourceEventHandler struct {
	ctx         context.Context
	processor   Processor
	bundleStore bundleStoreInterface
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
	metaObj, ok := obj.(meta_v1.Object)
	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			log.Printf("[REH] Delete event with unrecognized object type: %T", obj)
			return
		}
		metaObj, ok = tombstone.Obj.(meta_v1.Object)
		if !ok {
			log.Printf("[REH] Delete tombstone with unrecognized object type: %T", tombstone.Obj)
			return
		}
	}
	bundleName, namespace := getBundleNameAndNamespace(metaObj)
	if bundleName == "" { // No controller bundle found
		runtimeObj := metaObj.(runtime.Object)
		bundles, err := h.bundleStore.GetBundlesByObject(runtimeObj.GetObjectKind().GroupVersionKind().GroupKind(), namespace, metaObj.GetName())
		if err != nil {
			log.Printf("[REH] Failed to get bundles by object: %v", err)
			return
		}
		for _, bundle := range bundles {
			h.rebuildByName(namespace, bundle.Name, "deleted", metaObj)
		}
	} else {
		h.rebuildByName(namespace, bundleName, "deleted", metaObj)
	}
}

func (h *resourceEventHandler) rebuildByName(namespace, bundleName, addUpdateDelete string, obj interface{}) {
	if len(bundleName) == 0 {
		return
	}
	bundle, err := h.bundleStore.Get(namespace, bundleName)
	if err != nil {
		log.Printf("[REH][%s/%s] Failed to do bundle lookup: %v", namespace, bundleName, err)
		return
	}
	if bundle != nil {
		log.Printf("[REH][%s/%s] Rebuilding bundle because object %s was %s",
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
