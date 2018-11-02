package specchecker

import (
	"github.com/atlassian/smith/pkg/util"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/api/equality"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/diff"
	"k8s.io/apimachinery/pkg/util/sets"
)

type SpecCheck struct {
	KnownTypes map[schema.GroupKind]ObjectProcessor
}

func New(kts ...map[schema.GroupKind]ObjectProcessor) *SpecCheck {
	kt := make(map[schema.GroupKind]ObjectProcessor)
	for _, knownTypes := range kts {
		for knownGK, f := range knownTypes {
			if kt[knownGK] != nil {
				panic(errors.Errorf("GK specified more than once: %s", knownGK))
			}
			kt[knownGK] = f
		}
	}
	return &SpecCheck{
		KnownTypes: kt,
	}
}

func (sc *SpecCheck) CompareActualVsSpec(logger *zap.Logger, spec, actual runtime.Object) (*unstructured.Unstructured, bool /*match*/, string /* diff */, error) {
	specUnstr, err := util.RuntimeToUnstructured(spec)
	if err != nil {
		return nil, false, "", err
	}
	actualUnstr, err := util.RuntimeToUnstructured(actual)
	if err != nil {
		return nil, false, "", err
	}
	// Compare spec and existing resource
	return sc.compareActualVsSpec(logger, specUnstr, actualUnstr)
}

// compareActualVsSpec checks if actual resource satisfies the desired spec.
// If actual matches spec then actual is returned untouched otherwise an updated object is returned.
// Mutates spec (reuses parts of it).
func (sc *SpecCheck) compareActualVsSpec(logger *zap.Logger, spec, actual *unstructured.Unstructured) (*unstructured.Unstructured, bool /*match*/, string /* diff */, error) {
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
	gk := spec.GroupVersionKind().GroupKind()

	if processor, ok := sc.KnownTypes[gk]; ok {
		appliedSpec, err := processor.ApplySpec(&Context{Logger: logger}, spec, actualClone)
		if err != nil {
			return nil, false, "", errors.Wrap(err, "failed to apply specification to the actual object")
		}
		spec, err = util.RuntimeToUnstructured(appliedSpec)
		if err != nil {
			return nil, false, "", err
		}
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
		var difference string

		if util.IsSecret(spec) {
			difference = "Secret object has changed"
		} else {
			// return diff if not a secret
			difference = diff.ObjectDiff(updated.Object, actualClone.Object)
		}
		return updated, false, difference, nil
	}
	return actual, true, "", nil
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
		}
		return spec
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
		}
		return spec
	}

	for key, val := range spec {
		actual[key] = val
	}
	return actual
}
