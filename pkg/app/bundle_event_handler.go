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
	h.handle(obj)
}

func (h *bundleEventHandler) OnUpdate(oldObj, newObj interface{}) {
	h.handle(newObj)
}

func (h *bundleEventHandler) OnDelete(obj interface{}) {
	//		// TODO Somehow use finalizers to prevent direct deletion?
	//		// "No direct deletion" convention? Use ObjectMeta.DeletionTimestamp like Namespace does?
	//		// Somehow implement GC to do cleanup after bundle is deleted?
	//		// Maybe store bundle in annotation on each resource to help reconstruct the dependency graph for GC?
}

func (h *bundleEventHandler) handle(obj interface{}) {
	o, err := h.deepCopy(obj)
	if err != nil {
		log.Printf("[BEH] Failed to do deep copy of %#v: %v", obj, err)
		return
	}

	out := o.(*smith.Bundle)
	log.Printf("[BEH] Rebuilding %s/%s bundle because it was added/updated", out.Metadata.Namespace, out.Metadata.Name)
	if err = h.processor.Rebuild(h.ctx, out); err != nil && err != context.Canceled && err != context.DeadlineExceeded {
		log.Printf("[BEH] Error rebuilding bundle %s/%s: %v", out.Metadata.Namespace, out.Metadata.Name, err)
	}
}
