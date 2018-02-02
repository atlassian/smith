package util

import (
	"context"
	"time"

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

func ConvertType(scheme *runtime.Scheme, in, out runtime.Object) error {
	in = in.DeepCopyObject()
	if err := scheme.Convert(in, out, nil); err != nil {
		return err
	}
	// API machinery discards TypeMeta for typed objects. This is annoying.
	gvks, _, err := scheme.ObjectKinds(in)
	if err != nil {
		return err
	}
	out.GetObjectKind().SetGroupVersionKind(gvks[0])
	return nil
}

func RuntimeToUnstructured(scheme *runtime.Scheme, obj runtime.Object) (*unstructured.Unstructured, error) {
	out := &unstructured.Unstructured{}
	// TODO use scheme.Convert() when https://github.com/kubernetes/kubernetes/pull/59264 is fixed
	if err := ConvertType(scheme, obj, out); err != nil {
		return nil, err
	}
	return out, nil
}
