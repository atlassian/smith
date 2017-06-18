package controller

import (
	"github.com/atlassian/smith"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"
)

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
	// Get is a function that does a lookup of Bundle based on its namespace and name.
	Get(namespace, bundleName string) (*smith.Bundle, error)
	GetBundles(tprName string) ([]*smith.Bundle, error)
	GetBundlesByObject(gk schema.GroupKind, namespace, name string) ([]*smith.Bundle, error)
}
