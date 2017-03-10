package smith

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func DeepCopy_Bundle(in *Bundle, out *Bundle) error {
	if err := metav1.DeepCopy_v1_TypeMeta(&in.TypeMeta, &out.TypeMeta, nil); err != nil {
		return err
	}
	if err := metav1.DeepCopy_v1_ObjectMeta(&in.Metadata, &out.Metadata, nil); err != nil {
		return err
	}
	if err := DeepCopy_BundleSpec(&in.Spec, &out.Spec); err != nil {
		return err
	}
	return DeepCopy_BundleStatus(&in.Status, &out.Status)
}

func DeepCopy_BundleSpec(in *BundleSpec, out *BundleSpec) error {
	resources := make([]Resource, len(in.Resources))
	for i, r := range in.Resources {
		if err := DeepCopy_Resource(&r, &resources[i]); err != nil {
			return err
		}
	}
	out.Resources = resources
	return nil
}

func DeepCopy_Resource(in *Resource, out *Resource) error {
	out.Name = in.Name

	out.DependsOn = make([]DependencyRef, len(in.DependsOn))
	copy(out.DependsOn, in.DependsOn)

	return DeepCopy_Unstructured(&in.Spec, &out.Spec)
}

func DeepCopy_BundleStatus(in *BundleStatus, out *BundleStatus) error {
	*out = *in
	return nil
}

func DeepCopy_Unstructured(in *unstructured.Unstructured, out *unstructured.Unstructured) error {
	// TODO this is a shortcut. Do a proper deep copy instead.
	// https://github.com/kubernetes/kubernetes/issues/40657
	data, err := in.MarshalJSON()
	if err != nil {
		return err
	}
	return out.UnmarshalJSON(data)
}
