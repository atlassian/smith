package speccheck

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type SpecCleaner interface {
	Cleanup(spec, actual *unstructured.Unstructured) (updatedSpec *unstructured.Unstructured, err error)
}
