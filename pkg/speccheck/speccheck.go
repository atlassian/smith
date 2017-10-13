package speccheck

import (
	"fmt"
	"log"

	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	unstructured_conversion "k8s.io/apimachinery/pkg/conversion/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/diff"
)

type SpecCheck struct {
	Scheme *runtime.Scheme
	// Server fields cleanup
	Cleaner SpecCleaner
}

func (sc *SpecCheck) CompareActualVsSpec(spec, actual runtime.Object) (*unstructured.Unstructured, bool /*match*/, error) {
	// Apply defaults to the spec
	specUnstr, err := sc.applyDefaults(spec)
	if err != nil {
		return nil, false, fmt.Errorf("failed to apply defaults to object spec %v: %v", spec.GetObjectKind().GroupVersionKind(), err)
	}
	actualUnstr, err := sc.cloneAsUnstructured(actual)
	if err != nil {
		return nil, false, err
	}
	// Compare spec and existing resource
	return sc.compareActualVsSpec(specUnstr, actualUnstr)
}

func (sc *SpecCheck) applyDefaults(spec runtime.Object) (*unstructured.Unstructured, error) {
	gvk := spec.GetObjectKind().GroupVersionKind()
	if !sc.Scheme.Recognizes(gvk) {
		log.Printf("Unrecognized object type %v - not applying defaults", gvk)
		return sc.cloneAsUnstructured(spec)
	}
	var clone runtime.Object
	var err error
	if specUnstr, ok := spec.(*unstructured.Unstructured); ok {
		clone, err = sc.Scheme.New(gvk)
		if err != nil {
			return nil, err
		}
		if err = unstructured_conversion.DefaultConverter.FromUnstructured(specUnstr.Object, clone); err != nil {
			return nil, err
		}
	} else {
		clone, err = sc.Scheme.Copy(spec)
		if err != nil {
			return nil, err
		}
	}
	sc.Scheme.Default(clone)
	u, err := unstructured_conversion.DefaultConverter.ToUnstructured(clone)
	if err != nil {
		return nil, err
	}
	return &unstructured.Unstructured{
		Object: u,
	}, nil
}

// compareActualVsSpec checks if actual resource satisfies the desired spec.
// If actual matches spec then actual is returned untouched otherwise an updated object is returned.
// Mutates spec (reuses parts of it).
func (sc *SpecCheck) compareActualVsSpec(spec, actual *unstructured.Unstructured) (*unstructured.Unstructured, bool /*match*/, error) {
	upd, err := sc.Scheme.DeepCopy(actual)
	if err != nil {
		return nil, false, err
	}
	updated := upd.(*unstructured.Unstructured)
	delete(updated.Object, "status")

	actClone, err := sc.Scheme.DeepCopy(updated)
	if err != nil {
		return nil, false, err
	}
	actualClone := actClone.(*unstructured.Unstructured)

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

	// 2. Copy data from the spec
	for field, specValue := range spec.Object {
		switch field {
		case "kind", "apiVersion", "metadata", "status":
			continue
		}
		updated.Object[field] = specValue // using the value directly - we've made a copy up the stack so it's ok
	}

	// 3. Ignore fields managed by server
	updated, err = sc.Cleaner.Cleanup(updated, actualClone)
	if err != nil {
		return nil, false, err
	}

	// 4. Some stuff from ObjectMeta
	// TODO Ignores added annotations/labels. Should be configurable per-object and/or per-object kind?
	updated.SetName(spec.GetName())
	updated.SetLabels(spec.GetLabels())
	updated.SetAnnotations(processAnnotations(spec.GetAnnotations(), updated.GetAnnotations()))
	updated.SetOwnerReferences(spec.GetOwnerReferences()) // TODO Is this ok? Check that there is only one controller and it is THIS bundle
	updated.SetFinalizers(spec.GetFinalizers())           // TODO Is this ok?

	// Remove status to make sure ready checker will only detect readiness after resource controller has seen
	// the object.
	// Will be possible to implement it in a cleaner way once "status" is a separate sub-resource.
	// See https://github.com/kubernetes/kubernetes/issues/38113
	// Also ideally we don't want to clear the status at all but have a way to tell if controller has
	// observed the update yet. Like Generation/ObservedGeneration for built-in controllers.
	delete(updated.Object, "status")

	if !equality.Semantic.DeepEqual(updated.Object, actualClone.Object) {
		log.Printf("Objects are different: %s\nupdated: %v\nactualClone: %v",
			diff.ObjectReflectDiff(updated.Object, actualClone.Object), updated.Object, actualClone.Object)
		return updated, false, nil
	}
	return actual, true, nil
}

func (sc *SpecCheck) cloneAsUnstructured(obj runtime.Object) (*unstructured.Unstructured, error) {
	// ------ TODO this block is a workaround for https://github.com/kubernetes/kubernetes/issues/47889
	if _, ok := obj.(*unstructured.Unstructured); ok {
		clone, err := sc.Scheme.DeepCopy(obj)
		if err != nil {
			return nil, err
		}
		return clone.(*unstructured.Unstructured), nil
	}
	// ------
	u, err := unstructured_conversion.DefaultConverter.ToUnstructured(obj)
	if err != nil {
		return nil, err
	}
	return &unstructured.Unstructured{
		Object: u,
	}, nil
}

func processAnnotations(spec, actual map[string]string) map[string]string {
	for key, val := range spec {
		actual[key] = val
	}
	return actual
}
