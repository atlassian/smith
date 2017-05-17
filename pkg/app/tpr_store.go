package app

import (
	"github.com/atlassian/smith"
	"github.com/atlassian/smith/pkg/resources"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	extensions "k8s.io/client-go/pkg/apis/extensions/v1beta1"
)

type tprStore struct {
	store smith.ByNameStore
}

func (ts *tprStore) Get(resource schema.GroupKind) (*extensions.ThirdPartyResource, error) {
	tpr, exists, err := ts.store.Get(tprGVK, metav1.NamespaceNone, resources.GroupKindToTprName(resource))
	if err != nil || !exists {
		return nil, err
	}
	return tpr.(*extensions.ThirdPartyResource), nil
}
