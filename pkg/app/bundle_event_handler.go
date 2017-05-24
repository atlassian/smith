package app

import (
	"context"
	"log"

	"github.com/atlassian/smith"

	"k8s.io/client-go/tools/cache"
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
	_, ok := obj.(*smith.Bundle)
	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			log.Printf("[BEH] Delete event with unrecognized object type: %T", obj)
			return
		}
		obj, ok = tombstone.Obj.(*smith.Bundle)
		if !ok {
			log.Printf("[BEH] Delete tombstone with unrecognized object type: %T", tombstone.Obj)
			return
		}
	}
	h.handle(obj, "deleted")
}

func (h *bundleEventHandler) handle(obj interface{}, addUpdateDelete string) {
	o, err := h.deepCopy(obj)
	if err != nil {
		bundle := obj.(*smith.Bundle)
		log.Printf("[BEH][%s/%s] Failed to deep copy %T: %v", bundle.Namespace, bundle.Name, obj, err)
		return
	}

	bundle := o.(*smith.Bundle)
	log.Printf("[BEH][%s/%s] Rebuilding bundle because it was %s", bundle.Namespace, bundle.Name, addUpdateDelete)
	if err = h.processor.Rebuild(h.ctx, bundle); err != nil && err != context.Canceled && err != context.DeadlineExceeded {
		log.Printf("[BEH][%s/%s] Error rebuilding bundle: %v", bundle.Namespace, bundle.Name, err)
	}
}
