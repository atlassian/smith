package cleanup

import (
	"github.com/atlassian/smith/pkg/util"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Context includes objects used by different cleanup functions
type Context struct {
	Logger *zap.Logger
}

// SpecCleanup cleans the fields of the object which should be ignored.
// Each function is responsible for handling different versions of objects itself.
type SpecCleanup func(cleanupCtx *Context, spec, actual *unstructured.Unstructured) (updatedSpec runtime.Object, err error)

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

// Cleanup implements speccheck.SpecCleaner interface which will be called during speccheck.CompareActualVsSpec
func (sc *SpecCleaner) Cleanup(logger *zap.Logger, spec, actual *unstructured.Unstructured) (updatedSpec *unstructured.Unstructured, err error) {
	gk := spec.GroupVersionKind().GroupKind()

	if objCleanup, ok := sc.KnownTypes[gk]; ok {
		updated, err := objCleanup(&Context{Logger: logger}, spec, actual)
		if err != nil {
			return nil, errors.Wrap(err, "object cleanup failed")
		}
		return util.RuntimeToUnstructured(updated)
	}

	return spec, nil
}
