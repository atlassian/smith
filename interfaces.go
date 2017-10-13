package smith

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

type ByNameStore interface {
	Get(gvk schema.GroupVersionKind, namespace, name string) (obj runtime.Object, exists bool, err error)
}

type SmartClient interface {
	ForGVK(gvk schema.GroupVersionKind, namespace string) (dynamic.ResourceInterface, error)
}
