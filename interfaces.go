package smith

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

type DeepCopy func(src interface{}) (interface{}, error)

type ByNameStore interface {
	Get(gvk schema.GroupVersionKind, namespace, name string) (obj runtime.Object, exists bool, err error)
}

type SmartClient interface {
	ClientForGVK(gvk schema.GroupVersionKind, namespace string) (*dynamic.ResourceClient, error)
}
