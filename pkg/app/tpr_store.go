package app

import (
	"strings"

	extensions "k8s.io/client-go/pkg/apis/extensions/v1beta1"
	"k8s.io/client-go/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"
)

type TprStore struct {
	Store cache.Store
}

func (ts *TprStore) Get(resource schema.GroupKind) (*extensions.ThirdPartyResource, error) {
	// TODO do it properly - handle BlockLetters in the middle (turn them into dashes)
	name := strings.ToLower(resource.Kind) + "." + resource.Group
	// same key as client-go/tools/cache/store.MetaNamespaceKeyFunc
	// TODO replace with index to decouple from impl details
	tpr, exists, err := ts.Store.GetByKey(name)
	if err != nil || !exists {
		return nil, err
	}
	return tpr.(*extensions.ThirdPartyResource), nil
}
