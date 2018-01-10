package cleanup

import (
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// SpecCleanup cleans the fields of the object which should be ignored.
// Each function is responsible for handling different versions of objects itself.
type SpecCleanup func(spec, actual *unstructured.Unstructured) (updatedSpec *unstructured.Unstructured, err error)

type SpecCleaner struct {
	KnownTypes map[schema.GroupKind]SpecCleanup
}

func New(kts ...map[schema.GroupKind]SpecCleanup) *SpecCleaner {
	kt := make(map[schema.GroupKind]SpecCleanup)
	for _, knownTypes := range kts {
		for knownGK, f := range knownTypes {
			if kt[knownGK] != nil {
				panic(errors.Errorf("GK specified more than once: %s", knownGK))
			}
			kt[knownGK] = f
		}
	}
	return &SpecCleaner{
		KnownTypes: kt,
	}
}

func (oc *SpecCleaner) Cleanup(spec, actual *unstructured.Unstructured) (updatedSpec *unstructured.Unstructured, err error) {
	gk := spec.GroupVersionKind().GroupKind()

	if objCleanup, ok := oc.KnownTypes[gk]; ok {
		return objCleanup(spec, actual)
	}

	return spec, nil
}
