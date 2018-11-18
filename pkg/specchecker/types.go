package specchecker

import (
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type ObjectProcessor interface {
	// BeforeCreate pre-processes object specification and returns an updated version.
	BeforeCreate(ctx *Context, spec *unstructured.Unstructured) (runtime.Object /*updatedSpec*/, error)
	ApplySpec(ctx *Context, spec, actual *unstructured.Unstructured) (runtime.Object /*updatedSpec*/, error)
}

type Store interface {
	Get(gvk schema.GroupVersionKind, namespace, name string) (obj runtime.Object, exists bool, err error)
}

// Context includes objects used by different cleanup functions
type Context struct {
	Logger *zap.Logger
	Store  Store
}
