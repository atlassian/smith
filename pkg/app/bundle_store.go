package app

import (
	"github.com/atlassian/smith"

	"k8s.io/apimachinery/pkg/runtime"
)

type bundleStore struct {
	store  smith.ByNameStore
	scheme *runtime.Scheme
}

func (s *bundleStore) Get(namespace, bundleName string) (*smith.Bundle, error) {
	bundle, exists, err := s.store.Get(bundleGVK, namespace, bundleName)
	if err != nil || !exists {
		return nil, err
	}
	out, err := s.scheme.DeepCopy(bundle)
	if err != nil {
		return nil, err
	}

	return out.(*smith.Bundle), nil
}
