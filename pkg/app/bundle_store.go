package app

import (
	"github.com/atlassian/smith"

	"k8s.io/client-go/tools/cache"
)

type bundleStore struct {
	store cache.Store
}

func (s *bundleStore) Get(namespace, bundleName string) (*smith.Bundle, error) {
	bundle, exists, err := s.store.GetByKey(keyForBundle(namespace, bundleName))
	if err != nil || !exists {
		return nil, err
	}
	in := bundle.(*smith.Bundle)
	out := &smith.Bundle{}

	if err := smith.DeepCopy_Bundle(in, out); err != nil {
		return nil, err
	}
	return out, nil
}

// keyForBundle returns same key as client-go/tools/cache/store.MetaNamespaceKeyFunc
func keyForBundle(namespace, bundleName string) string {
	return namespace + "/" + bundleName
}
