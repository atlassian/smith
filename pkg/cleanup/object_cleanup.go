package cleanup

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type SpecCleaner interface {
	Cleanup(spec *unstructured.Unstructured, actual *unstructured.Unstructured) (updatedSpec *unstructured.Unstructured, err error)
}

// SpecCleanup cleans the fields of the object which should be ignored.
// Each function is responsible for handling different versions of objects itself.
type SpecCleanup func(spec *unstructured.Unstructured, actual *unstructured.Unstructured) (updatedSpec *unstructured.Unstructured, err error)

type SpecCleanerImpl struct {
	KnownTypes map[schema.GroupKind]SpecCleanup
}

func New(kts ...map[schema.GroupKind]SpecCleanup) SpecCleaner {
	kt := make(map[schema.GroupKind]SpecCleanup)
	for _, knownTypes := range kts {
		for knownGK, f := range knownTypes {
			if kt[knownGK] != nil {
				panic(fmt.Errorf("GK specified more than once: %s", knownGK))
			}
			kt[knownGK] = f
		}
	}
	return &SpecCleanerImpl{
		KnownTypes: kt,
	}
}

func (oc *SpecCleanerImpl) Cleanup(spec *unstructured.Unstructured, actual *unstructured.Unstructured) (updatedSpec *unstructured.Unstructured, err error) {
	gvk := spec.GroupVersionKind()
	gk := gvk.GroupKind()

	if gk.Kind == "" || gvk.Version == "" { // Group can be empty e.g. built-in objects like ConfigMap
		return nil, fmt.Errorf("object %q has empty kind/version: %v", spec.GetName(), gvk)
	}

	// Check if it is a known built-in resource
	if objCleanup, ok := oc.KnownTypes[gk]; ok {
		return objCleanup(spec, actual)
	}

	return spec, nil
}
