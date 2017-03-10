package app

import (
	"log"

	"github.com/atlassian/smith"
)

type bundleEventHandler struct {
	processor Processor
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
	in := obj.(*smith.Bundle)
	out := &smith.Bundle{}

	if err := smith.DeepCopy_Bundle(in, out); err != nil {
		log.Printf("[TEH] Failed to do deep copy of %#v: %v", in, err)
		return
	}
	log.Printf("[TEH] Rebuilding %s/%s bundle because it was added/updated", out.Metadata.Namespace, out.Metadata.Name)
	h.processor.Rebuild(out)
}
