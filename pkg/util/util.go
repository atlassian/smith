package util

import (
	"context"
	"time"

	"github.com/pkg/errors"
	core_v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

func Sleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	select {
	case <-ctx.Done():
		timer.Stop()
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// ConvertType should be used to convert to typed objects.
// If the in object is unstructured then it must have GVK set otherwise it must be
// recognizable by scheme or have the GVK set.
func ConvertType(scheme *runtime.Scheme, in, out runtime.Object) error {
	in = in.DeepCopyObject()
	if err := scheme.Convert(in, out, nil); err != nil {
		return err
	}
	gvkOut := out.GetObjectKind().GroupVersionKind()
	if gvkOut.Kind == "" || gvkOut.Version == "" { // Group can be empty
		// API machinery discards TypeMeta for typed objects. This is annoying.
		gvks, _, err := scheme.ObjectKinds(in)
		if err != nil {
			return err
		}
		out.GetObjectKind().SetGroupVersionKind(gvks[0])
	}
	return nil
}

// RuntimeToUnstructured can be used to convert any typed or unstructured object into
// an unstructured object. The obj must have GVK set.
func RuntimeToUnstructured(obj runtime.Object) (*unstructured.Unstructured, error) {
	gvk := obj.GetObjectKind().GroupVersionKind()
	if gvk.Kind == "" || gvk.Version == "" { // Group can be empty
		return nil, errors.Errorf("cannot convert %T to Unstructured: object Kind and/or object Version is empty", obj)
	}
	u, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj.DeepCopyObject())
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return &unstructured.Unstructured{
		Object: u,
	}, nil
}

func IsSecret(obj runtime.Object) bool {
	gvk := obj.GetObjectKind().GroupVersionKind()
	return gvk.Group == core_v1.GroupName && gvk.Kind == "Secret"
}
