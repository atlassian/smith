package speccheck

import (
	ctrlLogz "github.com/atlassian/ctrl/logz"
	"github.com/atlassian/smith/pkg/util"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	core_v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	unstructured.RemoveNestedField(updated.Object, "status")

	actualClone := updated.DeepCopy()

	// This is to ensure those fields do not exist in the underlying map whether they are nil or empty slices/maps
	// Because that is the behaviour of json omitempty
	trimEmptyField(actualClone, "metadata", "labels")
	trimEmptyField(actualClone, "metadata", "annotations")
	trimEmptyField(actualClone, "metadata", "ownerReferences")
	trimEmptyField(actualClone, "metadata", "finalizers")

	// Ignore fields managed by server, pre-process spec, etc
	spec, err := sc.Cleaner.Cleanup(spec, actualClone)
	if err != nil {
		return nil, false, errors.Wrap(err, "cleanup failed")
	}

	// Copy data from the spec
	for field, specValue := range spec.Object {
		switch field {
		case "kind", "apiVersion", "metadata", "status":
			continue
		}
		updated.Object[field] = specValue // using the value directly - we've made a copy up the stack so it's ok
	}

	// Some stuff from ObjectMeta
	updated.SetLabels(processLabels(spec.GetLabels()))
	updated.SetAnnotations(processAnnotations(spec.GetAnnotations(), updated.GetAnnotations()))
	updated.SetOwnerReferences(processOwnerReferences(spec.GetOwnerReferences()))
	updated.SetFinalizers(mergeFinalizers(spec.GetFinalizers(), updated.GetFinalizers()))

	// Remove status to make sure ready checker will only detect readiness after resource controller has seen
	// the object.
	// Will be possible to implement it in a cleaner way once "status" is a separate sub-resource.
	// See https://github.com/kubernetes/kubernetes/issues/38113
	// Also ideally we don't want to clear the status at all but have a way to tell if controller has
	// observed the update yet. Like Generation/ObservedGeneration for built-in controllers.
	unstructured.RemoveNestedField(updated.Object, "status")

	if !equality.Semantic.DeepEqual(updated.Object, actualClone.Object) {
		gvk := spec.GroupVersionKind()

		if gvk.Group == core_v1.GroupName && gvk.Kind == "Secret" {
			sc.Logger.Info("Objects are different: Secret object has changed", ctrlLogz.Object(spec))
			return updated, false, nil
		}

		sc.Logger.Sugar().Infof("Objects are different: %s", diff.ObjectDiff(updated.Object, actualClone.Object))
		return updated, false, nil
	}
	return actual, true, nil
}

// removes:
// - empty slice/map fields
// - typed nil slice/map fields
// - nil fields
func trimEmptyField(u *unstructured.Unstructured, fields ...string) {
	val, found, err := unstructured.NestedFieldNoCopy(u.Object, fields...)
	if err != nil || !found {
		return
	}
	remove := false
	switch typedVal := val.(type) {
	case map[string]interface{}:
		if len(typedVal) == 0 { // typed nil/empty map
			remove = true
		}
	case []interface{}:
		if len(typedVal) == 0 { // typed nil/empty slice
			remove = true
		}
	case nil: // untyped nil
		remove = true
	}
	if remove {
		unstructured.RemoveNestedField(u.Object, fields...)
	}
}

// TODO Is this ok? Check that there is only one controller and it is THIS bundle
func processOwnerReferences(spec []meta_v1.OwnerReference) []meta_v1.OwnerReference {
	if len(spec) == 0 {
		// return nil slice to make the field go away
		return nil
	}
	return spec
}

// TODO Nukes added labels. Should be configurable per-object and/or per-object kind?
func processLabels(spec map[string]string) map[string]string {
	if len(spec) == 0 {
		// return nil map to make the field go away
		return nil
	}
	return spec
}

func mergeFinalizers(spec, actual []string) []string {
	if len(actual) == 0 {
		if len(spec) == 0 {
			// nothing to update, return nil slice to make the field go away
			return nil
		} else {
			return spec
		}
	}
	finalizers := sets.NewString(spec...)
	finalizers.Insert(actual...)
	return finalizers.List() // Sorted list of all finalizers
}

func processAnnotations(spec, actual map[string]string) map[string]string {
	if len(actual) == 0 {
		if len(spec) == 0 {
			// nothing to update, return nil map to make the field go away
			return nil
		} else {
			return spec
		}
	}

	for key, val := range spec {
		actual[key] = val
	}
	return actual
}
