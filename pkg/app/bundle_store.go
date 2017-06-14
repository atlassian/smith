package app

import (
	"fmt"
	"strings"

	"github.com/atlassian/smith"
	"github.com/atlassian/smith/pkg/resources"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	ByTprNameIndex = "TprNameIndex"
	ByObjectIndex  = "ByObjectIndex"
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
	return s.getBundles(ByTprNameIndex, tprName)
}

// GetBundlesByObject returns bundles where a resource with specified GVK, namespace and name is defined.
func (s *bundleStore) GetBundlesByObject(gk schema.GroupKind, namespace, name string) ([]*smith.Bundle, error) {
	return s.getBundles(ByObjectIndex, ByObjectIndexKey(gk, namespace, name))
}

func (s *bundleStore) getBundles(indexName, indexKey string) ([]*smith.Bundle, error) {
	bundles, err := s.bundleByIndex(indexName, indexKey)
	if err != nil {
		return nil, err
	}
	result := make([]*smith.Bundle, 0, len(bundles))
	for _, bundle := range bundles {
		b, err := s.deepCopy(bundle)
		if err != nil {
			return nil, fmt.Errorf("failed to deep copy %T: %v", bundle, err)
		}
		result = append(result, b.(*smith.Bundle))
	}
	return result, nil
}

func byTprNameIndex(obj interface{}) ([]string, error) {
	bundle := obj.(*smith.Bundle)
	var result []string
	for _, resource := range bundle.Spec.Resources {
		gvk := resource.Spec.GetObjectKind().GroupVersionKind()
		if !strings.ContainsRune(gvk.Group, '.') {
			// TPR names are of form <kind>.<domain>.<tld> so there should be at least
			// one dot between domain and tld
			continue
		}
		result = append(result, resources.GroupKindToTprName(gvk.GroupKind()))
	}
	return result, nil
}

func byObjectIndex(obj interface{}) ([]string, error) {
	bundle := obj.(*smith.Bundle)
	result := make([]string, 0, len(bundle.Spec.Resources))
	for _, resource := range bundle.Spec.Resources {
		m, err := meta.Accessor(resource.Spec)
		if err != nil {
			return nil, fmt.Errorf("failed to get meta of object: %v", err)
		}
		result = append(result, ByObjectIndexKey(resource.Spec.GetObjectKind().GroupVersionKind().GroupKind(), bundle.Namespace, m.GetName()))
	}
	return result, nil
}

func ByObjectIndexKey(gk schema.GroupKind, namespace, name string) string {
	return fmt.Sprintf("%s/%s/%s/%s", gk.Group, gk.Kind, namespace, name)
}
