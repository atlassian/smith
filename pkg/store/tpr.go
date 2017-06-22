package store

import (
	"github.com/atlassian/smith"
	"github.com/atlassian/smith/pkg/resources"

	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ext_v1b1 "k8s.io/client-go/pkg/apis/extensions/v1beta1"
)

var (
	tprGVK = ext_v1b1.SchemeGroupVersion.WithKind("ThirdPartyResource")
)

type Tpr struct {
	Store smith.ByNameStore
}

func (ts *Tpr) Get(resource schema.GroupKind) (*ext_v1b1.ThirdPartyResource, error) {
	tpr, exists, err := ts.Store.Get(tprGVK, meta_v1.NamespaceNone, resources.GroupKindToTprName(resource))
	if err != nil || !exists {
		return nil, err
	}
	return tpr.(*ext_v1b1.ThirdPartyResource), nil
}
