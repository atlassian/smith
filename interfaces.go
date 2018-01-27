package smith

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type ByNameStore interface {
	Get(gvk schema.GroupVersionKind, namespace, name string) (obj runtime.Object, exists bool, err error)
}
