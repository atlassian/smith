package app

import (
	"github.com/atlassian/smith"
)

type bundleStore struct {
	store smith.ByNameStore
}

func (s *bundleStore) Get(namespace, bundleName string) (*smith.Bundle, error) {
	bundle, exists, err := s.store.Get(smith.BundleGVK, namespace, bundleName)
	if err != nil || !exists {
		return nil, err
	}
	return bundle.(*smith.Bundle), nil
}
