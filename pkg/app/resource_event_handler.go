package app

import (
	"context"
	"log"

	"github.com/atlassian/smith"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	oldTmplName, oldNamespace := getBundleNameAndNamespace(oldObj)

	newTmplName, newNamespace := getBundleNameAndNamespace(oldObj)

	if oldTmplName != newTmplName { // changed label on bundle
		h.rebuildByName(oldNamespace, oldTmplName, "updated", oldObj)
	}
	h.rebuildByName(newNamespace, newTmplName, "updated", newObj)
}

func (h *resourceEventHandler) OnDelete(obj interface{}) {
	bundleName, namespace := getBundleNameAndNamespace(obj)
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
			namespace, bundleName, obj.(metav1.Object).GetName(), addUpdateDelete)
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
	meta := obj.(metav1.Object)
	return meta.GetLabels()[smith.BundleNameLabel], meta.GetNamespace()
}
