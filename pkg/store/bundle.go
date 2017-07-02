package store

import (
	"fmt"
	"strings"

	"github.com/atlassian/smith"
	"github.com/atlassian/smith/pkg/resources"

	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"
)

const (
	byTprNameIndexName = "TprNameIndex"
	byObjectIndexName  = "ByObjectIndex"
)

type BundleStore struct {
	store         smith.ByNameStore
	bundleByIndex func(indexName, indexKey string) ([]interface{}, error)
	deepCopy      smith.DeepCopy
}

func NewBundle(bundleInf cache.SharedIndexInformer, store smith.ByNameStore, deepCopy smith.DeepCopy) (*BundleStore, error) {
	err := bundleInf.AddIndexers(cache.Indexers{
		byTprNameIndexName: byTprNameIndex,
		byObjectIndexName:  byObjectIndex,
	})
	if err != nil {
		return nil, err
	}
	return &BundleStore{
		store:         store,
		bundleByIndex: bundleInf.GetIndexer().ByIndex,
		deepCopy:      deepCopy,
	}, nil
}

// Get returns a bundle by its namespace and name.
// nil is returned if bundle does not exist.
func (s *BundleStore) Get(namespace, bundleName string) (*smith.Bundle, error) {
	bundle, exists, err := s.store.Get(smith.BundleGVK, namespace, bundleName)
	if err != nil || !exists {
		return nil, err
	}
	return bundle.(*smith.Bundle), nil
}

// GetBundles returns bundles where a resource declared via TPR with specified name is used.
func (s *BundleStore) GetBundles(tprName string) ([]*smith.Bundle, error) {
	return s.getBundles(byTprNameIndexName, tprName)
}

// GetBundlesByObject returns bundles where a resource with specified GVK, namespace and name is defined.
func (s *BundleStore) GetBundlesByObject(gk schema.GroupKind, namespace, name string) ([]*smith.Bundle, error) {
	return s.getBundles(byObjectIndexName, byObjectIndexKey(gk, namespace, name))
}

func (s *BundleStore) getBundles(indexName, indexKey string) ([]*smith.Bundle, error) {
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
		m := resource.Spec.(meta_v1.Object)
		result = append(result, byObjectIndexKey(resource.Spec.GetObjectKind().GroupVersionKind().GroupKind(), bundle.Namespace, m.GetName()))
	}
	return result, nil
}

func byObjectIndexKey(gk schema.GroupKind, namespace, name string) string {
	return fmt.Sprintf("%s/%s/%s/%s", gk.Group, gk.Kind, namespace, name)
}
