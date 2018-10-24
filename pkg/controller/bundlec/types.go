package bundlec

import (
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"go.uber.org/zap"
	apiext_v1b1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/cache"
)

type SpecCheck interface {
	CompareActualVsSpec(logger *zap.Logger, spec, actual runtime.Object) (updatedSpec *unstructured.Unstructured, match bool, diff string, err error)
}

type ReadyChecker interface {
	IsReady(*unstructured.Unstructured) (isReady, retriableError bool, e error)
}

type Store interface {
	Get(gvk schema.GroupVersionKind, namespace, name string) (obj runtime.Object, exists bool, err error)
	ObjectsControlledBy(namespace string, uid types.UID) ([]runtime.Object, error)
	AddInformer(schema.GroupVersionKind, cache.SharedIndexInformer) error
	RemoveInformer(schema.GroupVersionKind) bool
}

type BundleStore interface {
	// Get returns Bundle based on its namespace and name.
	Get(namespace, bundleName string) (*smith_v1.Bundle, error)
	// GetBundlesByCrd returns Bundles which have a resource defined by CRD.
	GetBundlesByCrd(*apiext_v1b1.CustomResourceDefinition) ([]*smith_v1.Bundle, error)
	// GetBundlesByObject returns Bundles which have a resource of a particular group/kind with a name in a namespace.
	GetBundlesByObject(gk schema.GroupKind, namespace, name string) ([]*smith_v1.Bundle, error)
}

type SmartClient interface {
	ForGVK(gvk schema.GroupVersionKind, namespace string) (dynamic.ResourceInterface, error)
}
