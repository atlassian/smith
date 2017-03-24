package app

import (
	"fmt"
	"sync"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"
)

const (
	ByNamespaceAndNameIndex string = "NamespaceNameIndex"
)

type Store struct {
	scheme *runtime.Scheme
	lock   sync.RWMutex
	stores map[schema.GroupVersionKind]cache.SharedIndexInformer
}

func NewStore(scheme *runtime.Scheme) *Store {
	return &Store{
		scheme: scheme,
		stores: make(map[schema.GroupVersionKind]cache.SharedIndexInformer),
	}
}

func (s *Store) AddInformer(gvk schema.GroupVersionKind, informer cache.SharedIndexInformer) {
	s.lock.Lock()
	defer s.lock.Unlock()
	if _, ok := s.stores[gvk]; ok {
		// This is a programming error hence panic
		panic(fmt.Errorf("Informer for %v is already registered", gvk))
	}
	err := informer.AddIndexers(cache.Indexers{
		ByNamespaceAndNameIndex: MetaNamespaceKeyFunc,
	})
	if err != nil {
		// Must never happen in our case
		panic(err)
	}
	s.stores[gvk] = informer
}

func (s *Store) RemoveInformer(gvk schema.GroupVersionKind) bool {
	s.lock.Lock()
	defer s.lock.Unlock()
	_, ok := s.stores[gvk]
	if ok {
		delete(s.stores, gvk)
	}
	return ok
}

// GetInformer gets an informer for the specified GVK.
// Returns false of no informer is registered.
func (s *Store) GetInformer(gvk schema.GroupVersionKind) (cache.SharedIndexInformer, bool) {
	s.lock.RLock()
	defer s.lock.RUnlock()
	informer, ok := s.stores[gvk]
	return informer, ok
}

// Get looks up object of specified GVK in the specified namespace by name.
// A deep copy of the object is returned so it is safe to modify it.
func (s *Store) Get(gvk schema.GroupVersionKind, namespace, name string) (obj runtime.Object, exists bool, e error) {
	informer, ok := s.GetInformer(gvk)
	if !ok {
		return nil, false, fmt.Errorf("no informer for %v is registered", gvk)
	}
	objs, err := informer.GetIndexer().ByIndex(ByNamespaceAndNameIndex, ByNamespaceAndNameIndexKey(namespace, name))
	if err != nil {
		return nil, false, err
	}
	switch len(objs) {
	case 0:
		return nil, false, nil
	case 1:
		obj, err := s.scheme.DeepCopy(objs[0])
		if err != nil {
			return nil, false, fmt.Errorf("failed to do deep copy of %#v: %v", objs[0], err)
		}
		return obj.(runtime.Object), true, nil
	default:
		// Must never happen
		panic(fmt.Errorf("multiple objects by namespace/name key for %v: %s", gvk, objs))
	}
}

// MetaNamespaceKeyFunc is a slightly modified cache.MetaNamespaceKeyFunc().
func MetaNamespaceKeyFunc(obj interface{}) ([]string, error) {
	if key, ok := obj.(cache.ExplicitKey); ok {
		return []string{string(key)}, nil
	}
	m, err := meta.Accessor(obj)
	if err != nil {
		return nil, fmt.Errorf("cannot get meta of object: %v", err)
	}
	return []string{ByNamespaceAndNameIndexKey(m.GetNamespace(), m.GetName())}, nil
}

func ByNamespaceAndNameIndexKey(namespace, name string) string {
	if namespace != "" {
		return namespace + "/" + name
	}
	return name
}
