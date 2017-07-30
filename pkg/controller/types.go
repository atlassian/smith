package controller

import (
	"github.com/atlassian/smith"

	apiext_v1b1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"
)

type SpecCheck interface {
	CompareActualVsSpec(spec, actual runtime.Object) (updatedSpec *unstructured.Unstructured, match bool, err error)
}

type ReadyChecker interface {
	IsReady(*unstructured.Unstructured) (isReady, retriableError bool, e error)
}

type Store interface {
	smith.ByNameStore
	GetObjectsForBundle(namespace, bundleName string) ([]runtime.Object, error)
	AddInformer(schema.GroupVersionKind, cache.SharedIndexInformer)
	RemoveInformer(schema.GroupVersionKind) bool
}

type BundleStore interface {
	// Get returns Bundle based on its namespace and name.
	Get(namespace, bundleName string) (*smith.Bundle, error)
	// GetBundlesByCrd returns Bundles which have a resource defined by CRD.
	GetBundlesByCrd(*apiext_v1b1.CustomResourceDefinition) ([]*smith.Bundle, error)
	// GetBundlesByObject returns Bundles which have a resource of a particular group/kind with a name in a namespace.
	GetBundlesByObject(gk schema.GroupKind, namespace, name string) ([]*smith.Bundle, error)
}
