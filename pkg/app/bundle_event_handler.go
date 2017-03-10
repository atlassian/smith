package app

import (
	"log"

	"github.com/atlassian/smith"
	"k8s.io/apimachinery/pkg/runtime"
)

type bundleEventHandler struct {
	processor Processor
	scheme    *runtime.Scheme
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

	out, err := h.scheme.DeepCopy(in)
	if err != nil {
		log.Printf("[TEH] Failed to do deep copy of %#v: %v", in, err)
		return
	}

	log.Printf("[TEH] Rebuilding %s/%s template because it was added/updated", out.(*smith.Bundle).Metadata.Namespace, out.(*smith.Bundle).Metadata.Name)
	h.processor.Rebuild(out.(*smith.Bundle))
}
