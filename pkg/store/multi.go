package store

import (
	"github.com/pkg/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"
)

const (
	ByNamespaceAndControllerUidIndex = "NamespaceUidIndex"
)

type Multi struct {
	MultiBasic
}

func NewMulti() *Multi {
	return &Multi{
		MultiBasic: *NewMultiBasic(),
	}
}

// AddInformer adds an Informer to the store.
// Can only be called with a not yet started informer. Otherwise bad things will happen.
func (s *Multi) AddInformer(gvk schema.GroupVersionKind, informer cache.SharedIndexInformer) error {
	s.mx.Lock()
	defer s.mx.Unlock()
	if _, ok := s.informers[gvk]; ok {
		return errors.New("informer is already registered")
	}
	f := informer.GetIndexer().GetIndexers()[ByNamespaceAndControllerUidIndex]
	if f == nil {
		// Informer does not have this index yet i.e. this is the first/sole multistore it is added to.
		err := informer.AddIndexers(cache.Indexers{
			ByNamespaceAndControllerUidIndex: byNamespaceAndControllerUidIndex,
		})
		if err != nil {
			return errors.WithStack(err)
		}
	}
	s.informers[gvk] = informer
	return nil
}

func (s *Multi) ObjectsControlledBy(namespace string, uid types.UID) ([]runtime.Object, error) {
	var result []runtime.Object
	indexKey := ByNamespaceAndControllerUidIndexKey(namespace, uid)
	for gvk, inf := range s.GetInformers() {
		objs, err := inf.GetIndexer().ByIndex(ByNamespaceAndControllerUidIndex, indexKey)
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

func byNamespaceAndControllerUidIndex(obj interface{}) ([]string, error) {
	if key, ok := obj.(cache.ExplicitKey); ok {
		return []string{string(key)}, nil
	}
	m := obj.(meta_v1.Object)
	ref := meta_v1.GetControllerOf(m)
	if ref != nil {
		return []string{ByNamespaceAndControllerUidIndexKey(m.GetNamespace(), ref.UID)}, nil
	}
	return nil, nil
}

func ByNamespaceAndControllerUidIndexKey(namespace string, uid types.UID) string {
	if namespace == meta_v1.NamespaceNone {
		return string(uid)
	}
	return namespace + "|" + string(uid)
}
