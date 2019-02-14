package store

import (
	"fmt"
	"strings"

	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/smith/pkg/plugin"
	apiext_v1b1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"
)

const (
	byCrdGroupKindIndexName = "ByCrdGroupKind"
	byObjectIndexName       = "ByObject"
)

type ByNameStore interface {
	Get(gvk schema.GroupVersionKind, namespace, name string) (obj runtime.Object, exists bool, err error)
}

type BundleStore struct {
	store            ByNameStore
	bundleByIndex    func(indexName, indexKey string) ([]interface{}, error)
	pluginContainers map[smith_v1.PluginName]plugin.Container
}

func NewBundle(bundleInf cache.SharedIndexInformer, store ByNameStore, pluginContainers map[smith_v1.PluginName]plugin.Container) (*BundleStore, error) {
	bs := &BundleStore{
		store:            store,
		bundleByIndex:    bundleInf.GetIndexer().ByIndex,
		pluginContainers: pluginContainers,
	}
	err := bundleInf.AddIndexers(cache.Indexers{
		byCrdGroupKindIndexName: bs.byCrdGroupKindIndex,
		byObjectIndexName:       bs.byObjectIndex,
	})
	if err != nil {
		return nil, err
	}
	return bs, nil
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
		result = append(result, bundle.(*smith_v1.Bundle).DeepCopy())
	}
	return result, nil
}

func (s *BundleStore) byCrdGroupKindIndex(obj interface{}) ([]string, error) {
	bundle := obj.(*smith_v1.Bundle)
	var result []string
	for _, resource := range bundle.Spec.Resources {
		var gvk schema.GroupVersionKind

		switch {
		case resource.Spec.Object != nil:
			gvk = resource.Spec.Object.GetObjectKind().GroupVersionKind()

		case resource.Spec.Plugin != nil:
			p, ok := s.pluginContainers[resource.Spec.Plugin.Name]
			if !ok {
				// Unknown plugin. Do not return error to avoid informer panicking
				continue
			}
			gvk = p.Plugin.Describe().GVK

		default:
			// Invalid object, ignore
			continue
		}
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

func (s *BundleStore) byObjectIndex(obj interface{}) ([]string, error) {
	bundle := obj.(*smith_v1.Bundle)
	result := make([]string, 0, len(bundle.Spec.Resources))
	for _, resource := range bundle.Spec.Resources {
		var gvk schema.GroupVersionKind
		var name string
		switch {
		case resource.Spec.Object != nil:
			gvk = resource.Spec.Object.GetObjectKind().GroupVersionKind()
			name = resource.Spec.Object.(meta_v1.Object).GetName()
		case resource.Spec.Plugin != nil:
			p, ok := s.pluginContainers[resource.Spec.Plugin.Name]
			if !ok {
				// Unknown plugin. Do not return error to avoid informer panicking
				continue
			}
			gvk = p.Plugin.Describe().GVK
			name = resource.Spec.Plugin.ObjectName
		default:
			// Invalid object, ignore
			continue
		}
		result = append(result, byObjectIndexKey(gvk.GroupKind(), bundle.Namespace, name))
	}
	return result, nil
}

func byObjectIndexKey(gk schema.GroupKind, namespace, name string) string {
	return fmt.Sprintf("%s/%s/%s/%s", gk.Group, gk.Kind, namespace, name)
}
