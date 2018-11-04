package speccheck

import (
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

type ObjectProcessor interface {
	ApplySpec(ctx *Context, spec, actual *unstructured.Unstructured) (runtime.Object /*updatedSpec*/, error)
}

// Context includes objects used by different cleanup functions
type Context struct {
	Logger *zap.Logger
}
