package app

import "github.com/atlassian/smith"

type templateEventHandler struct {
	processor Processor
}

// newTemplateEventHandler handles events for objects with Kind: Template.
func newTemplateEventHandler(processor Processor) *templateEventHandler {
	return &templateEventHandler{
		processor: processor,
	}
}

func (h *templateEventHandler) OnAdd(obj interface{}) {
	h.processor.Rebuild(obj.(*smith.Template))
}

func (h *templateEventHandler) OnUpdate(oldObj, newObj interface{}) {
	h.processor.Rebuild(newObj.(*smith.Template))
}

func (h *templateEventHandler) OnDelete(obj interface{}) {
	//		// TODO Somehow use finalizers to prevent direct deletion?
	//		// "No direct deletion" convention? Use ObjectMeta.DeletionTimestamp like Namespace does?
	//		// Somehow implement GC to do cleanup after template is deleted?
	//		// Maybe store template in annotation on each resource to help reconstruct the dependency graph for GC?
}
