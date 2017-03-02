package app

import (
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"
	extensions "k8s.io/client-go/pkg/apis/extensions/v1beta1"
	"k8s.io/client-go/tools/cache"
)

type tprStore struct {
	store cache.Store
}

func (ts *tprStore) Get(resource schema.GroupKind) (*extensions.ThirdPartyResource, error) {
	// TODO do it properly - handle BlockLetters in the middle (turn them into dashes)
	name := strings.ToLower(resource.Kind) + "." + resource.Group
	// same key as client-go/tools/cache/store.MetaNamespaceKeyFunc
	// TODO replace with index to decouple from impl details
	tpr, exists, err := ts.store.GetByKey(name)
	if err != nil || !exists {
		return nil, err
	}
	return tpr.(*extensions.ThirdPartyResource), nil
}
