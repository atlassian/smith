package speccheck

import (
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type SpecCleaner interface {
	Cleanup(logger *zap.Logger, spec, actual *unstructured.Unstructured) (updatedSpec *unstructured.Unstructured, err error)
}
