package app

import (
	"log"

	"github.com/atlassian/smith"

	"k8s.io/client-go/pkg/apis/meta/v1/unstructured"
)

// Name2Template is a function that does a lookup of Template based on its namespace and name.
type Name2Template func(namespace, tmplName string) (*smith.Template, error)

// tprInstanceEventHandler handles events for objects with various kinds, all are instance of some
// Third Party Resource.
type tprInstanceEventHandler struct {
	processor Processor
	name2tmpl Name2Template
}

func newTprInstanceEventHandler(processor Processor, name2tmpl Name2Template) *tprInstanceEventHandler {
	return &tprInstanceEventHandler{
		processor: processor,
		name2tmpl: name2tmpl,
	}
}

func (h *tprInstanceEventHandler) OnAdd(obj interface{}) {
	tmplName, namespace := getTemplateNameAndNamespace(obj)
	h.rebuildByName(namespace, tmplName)
}

func (h *tprInstanceEventHandler) OnUpdate(oldObj, newObj interface{}) {
	oldTmplName, oldNamespace := getTemplateNameAndNamespace(oldObj)

	newTmplName, newNamespace := getTemplateNameAndNamespace(oldObj)

	if oldTmplName != newTmplName { // changed label on template
		h.rebuildByName(oldNamespace, oldTmplName)
	}
	h.rebuildByName(newNamespace, newTmplName)
}

func (h *tprInstanceEventHandler) OnDelete(obj interface{}) {
	tmplName, namespace := getTemplateNameAndNamespace(obj)
	h.rebuildByName(namespace, tmplName)
}

func (h *tprInstanceEventHandler) rebuildByName(namespace, tmplName string) {
	if len(tmplName) == 0 {
		return
	}
	tmpl, err := h.name2tmpl(namespace, tmplName)
	if err != nil {
		log.Printf("Failed to do template lookup for %s/%s: %v", namespace, tmplName, err)
		return
	}
	if tmpl != nil {
		h.processor.Rebuild(tmpl)
	} else {
		// TODO template not found - handle deletion?
		// There may be a race between TPR instance informer and template informer in case of
		// connection loss. Because of that template informer might have stale cache without
		// a template for which TPR/resource informers already receive events. Because of that
		// template may be deleted erroneously (as it was not found in cache).
		// Need to handle this situation properly.
	}
}

func getTemplateNameAndNamespace(obj interface{}) (string, string) {
	tprInst := obj.(*unstructured.Unstructured)
	return tprInst.GetLabels()[smith.TemplateNameLabel], tprInst.GetNamespace()
}
