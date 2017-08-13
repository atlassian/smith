package store

import (
	"fmt"
	"strings"

	"github.com/atlassian/smith"
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"

	apiext_v1b1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"
)

const (
	byCrdGroupKindIndexName = "ByCrdGroupKind"
	byObjectIndexName       = "ByObject"
)

type BundleStore struct {
	store         smith.ByNameStore
	bundleByIndex func(indexName, indexKey string) ([]interface{}, error)
	deepCopy      smith.DeepCopy
}

func NewBundle(bundleInf cache.SharedIndexInformer, store smith.ByNameStore, deepCopy smith.DeepCopy) (*BundleStore, error) {
	err := bundleInf.AddIndexers(cache.Indexers{
		byCrdGroupKindIndexName: byCrdGroupKindIndex,
		byObjectIndexName:       byObjectIndex,
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
func (s *BundleStore) Get(namespace, bundleName string) (*smith_v1.Bundle, error) {
	bundle, exists, err := s.store.Get(smith_v1.BundleGVK, namespace, bundleName)
	if err != nil || !exists {
		return nil, err
	}
	return bundle.(*smith_v1.Bundle), nil
}

// GetBundlesByCrd returns Bundles which have a resource defined by CRD.
func (s *BundleStore) GetBundlesByCrd(crd *apiext_v1b1.CustomResourceDefinition) ([]*smith_v1.Bundle, error) {
	return s.getBundles(byCrdGroupKindIndexName, byCrdGroupKindIndexKey(crd.Spec.Group, crd.Spec.Names.Kind))
}

// GetBundlesByObject returns bundles where a resource with specified GVK, namespace and name is defined.
func (s *BundleStore) GetBundlesByObject(gk schema.GroupKind, namespace, name string) ([]*smith_v1.Bundle, error) {
	return s.getBundles(byObjectIndexName, byObjectIndexKey(gk, namespace, name))
}

func (s *BundleStore) getBundles(indexName, indexKey string) ([]*smith_v1.Bundle, error) {
	bundles, err := s.bundleByIndex(indexName, indexKey)
	if err != nil {
		return nil, err
	}
	result := make([]*smith_v1.Bundle, 0, len(bundles))
	for _, bundle := range bundles {
		b, err := s.deepCopy(bundle)
		if err != nil {
			return nil, fmt.Errorf("failed to deep copy %T: %v", bundle, err)
		}
		result = append(result, b.(*smith_v1.Bundle))
	}
	return result, nil
}

func byCrdGroupKindIndex(obj interface{}) ([]string, error) {
	bundle := obj.(*smith_v1.Bundle)
	var result []string
	for _, resource := range bundle.Spec.Resources {
		gvk := resource.Spec.GetObjectKind().GroupVersionKind()
		if strings.IndexByte(gvk.Group, '.') == -1 {
			// CRD names are of form <plural>.<domain>.<tld> so there should be at least
			// one dot between domain and tld
			continue
		}
		result = append(result, byCrdGroupKindIndexKey(gvk.Group, gvk.Kind))
	}
	return result, nil
}

func byCrdGroupKindIndexKey(group, kind string) string {
	return group + "/" + kind
}

func byObjectIndex(obj interface{}) ([]string, error) {
	bundle := obj.(*smith_v1.Bundle)
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
