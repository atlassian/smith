package app

import (
	"github.com/atlassian/smith"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
)

type bundleStore struct {
	store  cache.Store
	scheme *runtime.Scheme
}

func (s *bundleStore) Get(namespace, bundleName string) (*smith.Bundle, error) {
	bundle, exists, err := s.store.GetByKey(keyForBundle(namespace, bundleName))
	if err != nil || !exists {
		return nil, err
	}
	in := bundle.(*smith.Bundle)

	out, err := s.scheme.DeepCopy(in)
	if err != nil {
		return nil, err
	}

	return out.(*smith.Bundle), nil
}

// keyForBundle returns same key as client-go/tools/cache/store.MetaNamespaceKeyFunc
func keyForBundle(namespace, bundleName string) string {
	return namespace + "/" + bundleName
}
