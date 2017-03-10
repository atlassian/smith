package app

import (
	"log"

	"github.com/atlassian/smith"
	"k8s.io/apimachinery/pkg/conversion"
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

	c := conversion.NewCloner()
	out, err := c.DeepCopy(in)
	if err != nil {
		log.Printf("[TEH] Failed to do deep copy of %#v: %v", in, err)
		return
	}

	log.Printf("[TEH] Rebuilding %s/%s template because it was added/updated", out.(*smith.Template).Metadata.Namespace, out.(*smith.Template).Metadata.Name)
	h.processor.Rebuild(out.(*smith.Template))
}
