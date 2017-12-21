package store

import (
	"sync"

	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/pkg/errors"

	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"
)

const (
	ByNamespaceAndBundleNameIndex = "NamespaceBundleNameIndex"
)

type Multi struct {
	mx        sync.RWMutex // protects the map
	informers map[schema.GroupVersionKind]cache.SharedIndexInformer
}

func NewMulti() *Multi {
	return &Multi{
		informers: make(map[schema.GroupVersionKind]cache.SharedIndexInformer),
	}
}

// AddInformer adds an Informer to the store.
// Can only be called with a not yet started informer. Otherwise bad things will happen.
func (s *Multi) AddInformer(gvk schema.GroupVersionKind, informer cache.SharedIndexInformer) {
	s.mx.Lock()
	defer s.mx.Unlock()
	if _, ok := s.informers[gvk]; ok {
		// It is a programming error hence panic
		panic(errors.Errorf("Informer for %s is already registered", gvk))
	}
	err := informer.AddIndexers(cache.Indexers{
		ByNamespaceAndBundleNameIndex: byNamespaceAndBundleNameIndex,
	})
	if err != nil {
		// Must never happen in our case
		panic(err)
	}
	s.informers[gvk] = informer
}

func (s *Multi) RemoveInformer(gvk schema.GroupVersionKind) bool {
	s.mx.Lock()
	defer s.mx.Unlock()
	_, ok := s.informers[gvk]
	if ok {
		delete(s.informers, gvk)
	}
	return ok
}

// GetInformers gets all registered Informers.
func (s *Multi) GetInformers() map[schema.GroupVersionKind]cache.SharedIndexInformer {
	s.mx.RLock()
	defer s.mx.RUnlock()
	informers := make(map[schema.GroupVersionKind]cache.SharedIndexInformer, len(s.informers))
	for gvk, inf := range s.informers {
		informers[gvk] = inf
	}
	return informers
}

// Get looks up object of specified GVK in the specified namespace by name.
// A deep copy of the object is returned so it is safe to modify it.
func (s *Multi) Get(gvk schema.GroupVersionKind, namespace, name string) (obj runtime.Object, exists bool, e error) {
	var informer cache.SharedIndexInformer
	func() {
		s.mx.RLock()
		defer s.mx.RUnlock()
		informer = s.informers[gvk]
	}()
	if informer == nil {
		return nil, false, errors.Errorf("no informer for %s is registered", gvk)
	}
	return s.getFromIndexer(informer.GetIndexer(), gvk, namespace, name)
}

func (s *Multi) getFromIndexer(indexer cache.Indexer, gvk schema.GroupVersionKind, namespace, name string) (runtime.Object, bool /*exists */, error) {
	obj, exists, err := indexer.GetByKey(ByNamespaceAndNameIndexKey(namespace, name))
	if err != nil || !exists {
		return nil, exists, err
	}
	ro := obj.(runtime.Object).DeepCopyObject()
	ro.GetObjectKind().SetGroupVersionKind(gvk) // Objects from type-specific informers don't have GVK set
	return ro, true, nil
}

func (s *Multi) GetObjectsForBundle(namespace, bundleName string) ([]runtime.Object, error) {
	var result []runtime.Object
	indexKey := ByNamespaceAndBundleNameIndexKey(namespace, bundleName)
	for gvk, inf := range s.GetInformers() {
		objs, err := inf.GetIndexer().ByIndex(ByNamespaceAndBundleNameIndex, indexKey)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get objects for bundle from %s informer", gvk)
		}
		for _, obj := range objs {
			ro := obj.(runtime.Object).DeepCopyObject()
			ro.GetObjectKind().SetGroupVersionKind(gvk) // Objects from type-specific informers don't have GVK set
			result = append(result, ro)
		}
	}
	return result, nil
}

func byNamespaceAndBundleNameIndex(obj interface{}) ([]string, error) {
	if key, ok := obj.(cache.ExplicitKey); ok {
		return []string{string(key)}, nil
	}
	m := obj.(meta_v1.Object)
	ref := meta_v1.GetControllerOf(m)
	if ref != nil && ref.APIVersion == smith_v1.BundleResourceGroupVersion && ref.Kind == smith_v1.BundleResourceKind {
		return []string{ByNamespaceAndBundleNameIndexKey(m.GetNamespace(), ref.Name)}, nil
	}
	return nil, nil
}

func ByNamespaceAndBundleNameIndexKey(namespace, bundleName string) string {
	return namespace + "|" + bundleName // Different separator to avoid clashes with ByNamespaceAndNameIndexKey
}

func ByNamespaceAndNameIndexKey(namespace, name string) string {
	if namespace == meta_v1.NamespaceNone {
		return name
	}
	return namespace + "/" + name
}
