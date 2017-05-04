package app

import (
	"log"
	"strings"

	"github.com/atlassian/smith"
	"github.com/atlassian/smith/pkg/resources"
)

const (
	ByTprNameIndex = "TprNameIndex"
)

type byIndex func(indexName, indexKey string) ([]interface{}, error)

type bundleStore struct {
	store         smith.ByNameStore
	bundleByIndex byIndex
	deepCopy      smith.DeepCopy
}

// Get returns a bundle by its namespace and name.
// nil is returned if bundle does not exist.
func (s *bundleStore) Get(namespace, bundleName string) (*smith.Bundle, error) {
	bundle, exists, err := s.store.Get(smith.BundleGVK, namespace, bundleName)
	if err != nil || !exists {
		return nil, err
	}
	return bundle.(*smith.Bundle), nil
}

// GetBundles returns bundles where a resource declared via TPR with specified name is used.
func (s *bundleStore) GetBundles(tprName string) ([]*smith.Bundle, error) {
	bundles, err := s.bundleByIndex(ByTprNameIndex, tprName)
	if err != nil {
		return nil, err
	}
	result := make([]*smith.Bundle, 0, len(bundles))
	for _, bundle := range bundles {
		b, err := s.deepCopy(bundle)
		if err != nil {
			log.Printf("Failed to deep copy %T: %v", bundle, err)
			continue
		}
		result = append(result, b.(*smith.Bundle))
	}
	return result, nil
}

func byTprNameIndex(obj interface{}) ([]string, error) {
	bundle := obj.(*smith.Bundle)
	var result []string
	for _, resource := range bundle.Spec.Resources {
		gvk := resource.Spec.GroupVersionKind()
		if !strings.ContainsRune(gvk.Group, '.') {
			// TPR names are of form <kind>.<domain>.<tld> so there should be at least
			// one dot between domain and tld
			continue
		}
		result = append(result, resources.GroupKindToTprName(gvk.GroupKind()))
	}
	return result, nil
}
