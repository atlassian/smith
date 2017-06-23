package cleanup

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type ObjectCleaner interface {
	Cleanup(obj *unstructured.Unstructured) (updatedObj *unstructured.Unstructured, err error)
}

// Cleanup cleans the fields of the object which should be ignored.
// Each function is responsible for handling different versions of objects itself.
type Cleanup func(obj *unstructured.Unstructured) (updatedObj *unstructured.Unstructured, err error)

type ObjectCleanerImpl struct {
	KnownTypes map[schema.GroupKind]Cleanup
}

func New(kts ...map[schema.GroupKind]Cleanup) ObjectCleaner {
	kt := make(map[schema.GroupKind]Cleanup)
	for _, knownTypes := range kts {
		for knownGK, f := range knownTypes {
			if kt[knownGK] != nil {
				panic(fmt.Errorf("GK specified more than once: %s", knownGK))
			}
			kt[knownGK] = f
		}
	}
	return &ObjectCleanerImpl{
		KnownTypes: kt,
	}
}

func (oc *ObjectCleanerImpl) Cleanup(obj *unstructured.Unstructured) (updatedObj *unstructured.Unstructured, err error) {
	gvk := obj.GroupVersionKind()
	gk := gvk.GroupKind()

	if gk.Kind == "" || gvk.Version == "" { // Group can be empty e.g. built-in objects like ConfigMap
		return nil, fmt.Errorf("object %q has empty kind/version: %v", obj.GetName(), gvk)
	}

	// Check if it is a known built-in resource
	if objCleanup, ok := oc.KnownTypes[gk]; ok {
		return objCleanup(obj)
	}

	return obj, nil
}
