package store

import (
	"sync"

	"github.com/pkg/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"
)

type MultiBasic struct {
	mx        sync.RWMutex // protects the map
	informers map[schema.GroupVersionKind]cache.SharedIndexInformer
}

func NewMultiBasic() *MultiBasic {
	return &MultiBasic{
		informers: make(map[schema.GroupVersionKind]cache.SharedIndexInformer),
	}
}

// AddInformer adds an Informer to the store.
func (s *MultiBasic) AddInformer(gvk schema.GroupVersionKind, informer cache.SharedIndexInformer) error {
	s.mx.Lock()
	defer s.mx.Unlock()
	if _, ok := s.informers[gvk]; ok {
		return errors.New("informer is already registered")
	}
	s.informers[gvk] = informer
	return nil
}

func (s *MultiBasic) RemoveInformer(gvk schema.GroupVersionKind) bool {
	s.mx.Lock()
	defer s.mx.Unlock()
	_, ok := s.informers[gvk]
	if ok {
		delete(s.informers, gvk)
	}
	return ok
}

// GetInformers gets all registered Informers.
func (s *MultiBasic) GetInformers() map[schema.GroupVersionKind]cache.SharedIndexInformer {
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
func (s *MultiBasic) Get(gvk schema.GroupVersionKind, namespace, name string) (obj runtime.Object, exists bool, e error) {
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

func (s *MultiBasic) getFromIndexer(indexer cache.Indexer, gvk schema.GroupVersionKind, namespace, name string) (runtime.Object, bool /*exists */, error) {
	obj, exists, err := indexer.GetByKey(ByNamespaceAndNameIndexKey(namespace, name))
	if err != nil || !exists {
		return nil, exists, err
	}
	ro := obj.(runtime.Object).DeepCopyObject()
	ro.GetObjectKind().SetGroupVersionKind(gvk) // Objects from type-specific informers don't have GVK set
	return ro, true, nil
}

func ByNamespaceAndNameIndexKey(namespace, name string) string {
	if namespace == meta_v1.NamespaceNone {
		return name
	}
	return namespace + "/" + name
}
