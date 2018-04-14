package speccheck

import (
	"github.com/atlassian/smith/pkg/util"
	"github.com/atlassian/smith/pkg/util/logz"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	core_v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/diff"
	"k8s.io/apimachinery/pkg/util/sets"
)

type SpecCheck struct {
	Logger *zap.Logger
	// Server fields cleanup
	Cleaner SpecCleaner
}

func (sc *SpecCheck) CompareActualVsSpec(spec, actual runtime.Object) (*unstructured.Unstructured, bool /*match*/, error) {
	specUnstr, err := util.RuntimeToUnstructured(spec)
	if err != nil {
		return nil, false, err
	}
	actualUnstr, err := util.RuntimeToUnstructured(actual)
	if err != nil {
		return nil, false, err
	}
	// Compare spec and existing resource
	return sc.compareActualVsSpec(specUnstr, actualUnstr)
}

// compareActualVsSpec checks if actual resource satisfies the desired spec.
// If actual matches spec then actual is returned untouched otherwise an updated object is returned.
// Mutates spec (reuses parts of it).
func (sc *SpecCheck) compareActualVsSpec(spec, actual *unstructured.Unstructured) (*unstructured.Unstructured, bool /*match*/, error) {
	updated := actual.DeepCopy()
	delete(updated.Object, "status")

	actualClone := updated.DeepCopy()

	// This is to ensure those fields actually exist in underlying map whether they are nil or empty slices/map
	actualClone.SetKind(spec.GetKind())             // Objects from type-specific informers don't have kind/api version
	actualClone.SetAPIVersion(spec.GetAPIVersion()) // Objects from type-specific informers don't have kind/api version
	actualClone.SetName(actualClone.GetName())
	actualClone.SetLabels(actualClone.GetLabels())
	actualClone.SetAnnotations(actualClone.GetAnnotations())
	actualClone.SetOwnerReferences(actualClone.GetOwnerReferences())
	actualClone.SetFinalizers(actualClone.GetFinalizers())

	// 1. TypeMeta
	updated.SetKind(spec.GetKind())
	updated.SetAPIVersion(spec.GetAPIVersion())

	// 2. Ignore fields managed by server, pre-process spec, etc
	spec, err := sc.Cleaner.Cleanup(spec, actualClone)
	if err != nil {
		return nil, false, errors.Wrapf(err, "cleanup failed")
	}

	// 3. Copy data from the spec
	for field, specValue := range spec.Object {
		switch field {
		case "kind", "apiVersion", "metadata", "status":
			continue
		}
		updated.Object[field] = specValue // using the value directly - we've made a copy up the stack so it's ok
	}

	// 4. Some stuff from ObjectMeta
	// TODO Ignores added annotations/labels. Should be configurable per-object and/or per-object kind?
	updated.SetName(spec.GetName())
	updated.SetLabels(spec.GetLabels())
	updated.SetAnnotations(processAnnotations(spec.GetAnnotations(), updated.GetAnnotations()))
	updated.SetOwnerReferences(spec.GetOwnerReferences()) // TODO Is this ok? Check that there is only one controller and it is THIS bundle

	finalizers := sets.NewString(updated.GetFinalizers()...)
	finalizers.Insert(spec.GetFinalizers()...)
	updated.SetFinalizers(finalizers.List()) // Sorted list of all finalizers

	// Remove status to make sure ready checker will only detect readiness after resource controller has seen
	// the object.
	// Will be possible to implement it in a cleaner way once "status" is a separate sub-resource.
	// See https://github.com/kubernetes/kubernetes/issues/38113
	// Also ideally we don't want to clear the status at all but have a way to tell if controller has
	// observed the update yet. Like Generation/ObservedGeneration for built-in controllers.
	delete(updated.Object, "status")

	if !equality.Semantic.DeepEqual(updated.Object, actualClone.Object) {
		gvk := spec.GroupVersionKind()

		if gvk.Group == core_v1.GroupName && gvk.Kind == "Secret" {
			sc.Logger.Info("Objects are different: Secret object has changed", logz.Object(spec))
			return updated, false, nil
		}

		sc.Logger.Sugar().Infof("Objects are different: %s",
			diff.ObjectReflectDiff(updated.Object, actualClone.Object))
		return updated, false, nil
	}
	return actual, true, nil
}

func processAnnotations(spec, actual map[string]string) map[string]string {
	if actual == nil {
		actual = make(map[string]string, len(spec))
	}

	for key, val := range spec {
		actual[key] = val
	}
	return actual
}
