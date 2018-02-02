package cleanup

import (
	"github.com/atlassian/smith/pkg/util"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// SpecCleanup cleans the fields of the object which should be ignored.
// Each function is responsible for handling different versions of objects itself.
type SpecCleanup func(scheme *runtime.Scheme, spec, actual *unstructured.Unstructured) (updatedSpec runtime.Object, err error)

type SpecCleaner struct {
	Scheme     *runtime.Scheme
	KnownTypes map[schema.GroupKind]SpecCleanup
}

func New(scheme *runtime.Scheme, kts ...map[schema.GroupKind]SpecCleanup) *SpecCleaner {
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
		Scheme:     scheme,
		KnownTypes: kt,
	}
}

func (sc *SpecCleaner) Cleanup(spec, actual *unstructured.Unstructured) (updatedSpec *unstructured.Unstructured, err error) {
	gk := spec.GroupVersionKind().GroupKind()

	if objCleanup, ok := sc.KnownTypes[gk]; ok {
		updated, err := objCleanup(sc.Scheme, spec, actual)
		if err != nil {
			return nil, errors.Wrap(err, "object cleanup failed")
		}
		return util.RuntimeToUnstructured(sc.Scheme, updated)
	}

	return spec, nil
}
