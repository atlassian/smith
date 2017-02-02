package app

import (
	extensions "k8s.io/client-go/pkg/apis/extensions/v1beta1"
	"k8s.io/client-go/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"
)

type TprStore struct {
	Store cache.Store
}

func (ts *TprStore) Get(resource schema.GroupKind) (*extensions.ThirdPartyResource, error) {
	// TODO implement
	return nil, nil
}

// keyForTpr returns same key as client-go/tools/cache/store.MetaNamespaceKeyFunc
// TODO replace with index to decouple from impl details
func keyForTpr(namespace, tmplName string) string {
	return namespace + "/" + tmplName
}
