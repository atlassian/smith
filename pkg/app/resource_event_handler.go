package app

import (
	"log"

	"github.com/atlassian/smith"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Name2Bundle is a function that does a lookup of Bundle based on its namespace and name.
type Name2Bundle func(namespace, bundleName string) (*smith.Bundle, error)

// resourceEventHandler handles events for objects with various kinds.
type resourceEventHandler struct {
	processor   Processor
	name2bundle Name2Bundle
}

func (h *resourceEventHandler) OnAdd(obj interface{}) {
	bundleName, namespace := getBundleNameAndNamespace(obj)
	h.rebuildByName(namespace, bundleName, obj)
}

func (h *resourceEventHandler) OnUpdate(oldObj, newObj interface{}) {
	oldTmplName, oldNamespace := getBundleNameAndNamespace(oldObj)

	newTmplName, newNamespace := getBundleNameAndNamespace(oldObj)

	if oldTmplName != newTmplName { // changed label on bundle
		h.rebuildByName(oldNamespace, oldTmplName, oldObj)
	}
	h.rebuildByName(newNamespace, newTmplName, newObj)
}

func (h *resourceEventHandler) OnDelete(obj interface{}) {
	bundleName, namespace := getBundleNameAndNamespace(obj)
	h.rebuildByName(namespace, bundleName, obj)
}

func (h *resourceEventHandler) rebuildByName(namespace, bundleName string, obj interface{}) {
	if len(bundleName) == 0 {
		return
	}
	bundle, err := h.name2bundle(namespace, bundleName)
	if err != nil {
		log.Printf("[REH] Failed to do bundle lookup for %s/%s: %v", namespace, bundleName, err)
		return
	}
	if bundle != nil {
		log.Printf("[REH] Rebuilding %s/%s bundle because of resource %s add/update/delete", namespace, bundleName, obj.(*unstructured.Unstructured).GetName())
		h.processor.Rebuild(bundle)
	} else {
		// TODO bundle not found - handle deletion?
		// There may be a race between TPR instance informer and bundle informer in case of
		// connection loss. Because of that bundle informer might have stale cache without
		// a bundle for which TPR/resource informers already receive events. Because of that
		// bundle may be deleted erroneously (as it was not found in cache).
		// Need to handle this situation properly.
	}
}

func getBundleNameAndNamespace(obj interface{}) (string, string) {
	tprInst := obj.(*unstructured.Unstructured)
	return tprInst.GetLabels()[smith.BundleNameLabel], tprInst.GetNamespace()
}
