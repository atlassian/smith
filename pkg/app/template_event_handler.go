package app

import (
	"log"

	"github.com/atlassian/smith"
)

type templateEventHandler struct {
	processor Processor
}

func (h *templateEventHandler) OnAdd(obj interface{}) {
	h.handle(obj)
}

func (h *templateEventHandler) OnUpdate(oldObj, newObj interface{}) {
	h.handle(newObj)
}

func (h *templateEventHandler) OnDelete(obj interface{}) {
	//		// TODO Somehow use finalizers to prevent direct deletion?
	//		// "No direct deletion" convention? Use ObjectMeta.DeletionTimestamp like Namespace does?
	//		// Somehow implement GC to do cleanup after template is deleted?
	//		// Maybe store template in annotation on each resource to help reconstruct the dependency graph for GC?
}

func (h *templateEventHandler) handle(obj interface{}) {
	in := obj.(*smith.Template)
	out := &smith.Template{}

	if err := smith.DeepCopy_Template(in, out); err != nil {
		log.Printf("[TEH] Failed to do deep copy of %#v: %v", in, err)
		return
	}
	log.Printf("[TEH] Rebuilding %s/%s template because it was added/updated", out.Metadata.Namespace, out.Metadata.Name)
	h.processor.Rebuild(out)
}
