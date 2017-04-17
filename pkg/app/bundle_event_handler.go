package app

import (
	"context"
	"log"

	"github.com/atlassian/smith"
)

type bundleEventHandler struct {
	ctx       context.Context
	processor Processor
	deepCopy  smith.DeepCopy
}

func (h *bundleEventHandler) OnAdd(obj interface{}) {
	h.handle(obj, "added")
}

func (h *bundleEventHandler) OnUpdate(oldObj, newObj interface{}) {
	h.handle(newObj, "updated")
}

func (h *bundleEventHandler) OnDelete(obj interface{}) {
	//		// TODO Somehow use finalizers to prevent direct deletion?
	//		// "No direct deletion" convention? Use ObjectMeta.DeletionTimestamp like Namespace does?
	//		// Somehow implement GC to do cleanup after bundle is deleted?
	//		// Maybe store bundle in annotation on each resource to help reconstruct the dependency graph for GC?
}

func (h *bundleEventHandler) handle(obj interface{}, addUpdate string) {
	o, err := h.deepCopy(obj)
	if err != nil {
		bundle := o.(*smith.Bundle)
		log.Printf("[BEH][%s/%s] Failed to deep copy %T: %v", bundle.Metadata.Namespace, bundle.Metadata.Name, obj, err)
		return
	}

	bundle := o.(*smith.Bundle)
	meta := bundle.Metadata
	log.Printf("[BEH][%s/%s] Rebuilding bundle because it was %s", meta.Namespace, meta.Name, addUpdate)
	if err = h.processor.Rebuild(h.ctx, bundle); err != nil && err != context.Canceled && err != context.DeadlineExceeded {
		log.Printf("[BEH][%s/%s] Error rebuilding bundle: %v", meta.Namespace, meta.Name, err)
	}
}
