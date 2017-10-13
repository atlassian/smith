package store

import (
	"fmt"

	apiext_v1b1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"
)

const (
	byGroupKindIndexName = "ByGroupKind"
)

type Crd struct {
	byIndex func(indexName, indexKey string) ([]interface{}, error)
}

func NewCrd(crdInf cache.SharedIndexInformer) (*Crd, error) {
	err := crdInf.AddIndexers(cache.Indexers{
		byGroupKindIndexName: byGroupKindIndex,
	})
	if err != nil {
		return nil, err
	}
	return &Crd{
		byIndex: crdInf.GetIndexer().ByIndex,
	}, nil
}

// Get returns the CRD that defines the resource of provided group and kind.
func (s *Crd) Get(resource schema.GroupKind) (*apiext_v1b1.CustomResourceDefinition, error) {
	objs, err := s.byIndex(byGroupKindIndexName, byGroupKindIndexKey(resource.Group, resource.Kind))
	if err != nil {
		return nil, err
	}
	switch len(objs) {
	case 0:
		return nil, nil
	case 1:
		crd := objs[0].(*apiext_v1b1.CustomResourceDefinition).DeepCopy()
		// Objects from type-specific informers don't have GVK set
		crd.Kind = "CustomResourceDefinition"
		crd.APIVersion = apiext_v1b1.SchemeGroupVersion.String()
		return crd, nil
	default:
		// Must never happen
		panic(fmt.Errorf("multiple CRDs by group %q and kind %q: %s", resource.Group, resource.Kind, objs))
	}
}

func byGroupKindIndex(obj interface{}) ([]string, error) {
	crd := obj.(*apiext_v1b1.CustomResourceDefinition)
	return []string{byGroupKindIndexKey(crd.Spec.Group, crd.Spec.Names.Kind)}, nil
}

func byGroupKindIndexKey(group, kind string) string {
	return group + "/" + kind
}
