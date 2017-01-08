package app

import (
	"github.com/atlassian/smith"

	"k8s.io/client-go/pkg/runtime"
)

// tprInstanceEventHandler handles events for objects with various kinds, all are instance of some
// Third Party Resource.
type tprInstanceEventHandler struct {
	processor Processor
}

func newTprInstanceEventHandler(processor Processor) *tprInstanceEventHandler {
	return &tprInstanceEventHandler{
		processor: processor,
	}
}

func (h *tprInstanceEventHandler) OnAdd(obj interface{}) {
	h.handle(obj)
}

func (h *tprInstanceEventHandler) OnUpdate(oldObj, newObj interface{}) {
	oldTemplateName, oldNamespace := getTemplateNameAndNamespace(oldObj)

	newTemplateName, newNamespace := getTemplateNameAndNamespace(oldObj)

	if oldTemplateName != newTemplateName { // changed label on template
		h.processor.RebuildByName(oldNamespace, oldTemplateName)
	}
	h.processor.RebuildByName(newNamespace, newTemplateName)
}

func (h *tprInstanceEventHandler) OnDelete(obj interface{}) {
	h.handle(obj)
}

func (h *tprInstanceEventHandler) handle(obj interface{}) {
	templateName, namespace := getTemplateNameAndNamespace(obj)
	if len(templateName) != 0 {
		h.processor.RebuildByName(namespace, templateName)
	}
}

func getTemplateNameAndNamespace(obj interface{}) (string, string) {
	tprInst := obj.(*runtime.Unstructured)
	return tprInst.GetLabels()[smith.TemplateNameLabel], tprInst.GetNamespace()
}
