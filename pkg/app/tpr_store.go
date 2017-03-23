package app

import (
	"strings"

	"github.com/atlassian/smith"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	extensions "k8s.io/client-go/pkg/apis/extensions/v1beta1"
)

type tprStore struct {
	store smith.ByNameStore
}

func (ts *tprStore) Get(resource schema.GroupKind) (*extensions.ThirdPartyResource, error) {
	// TODO do it properly - handle BlockLetters in the middle (turn them into dashes)
	name := strings.ToLower(resource.Kind) + "." + resource.Group
	tpr, exists, err := ts.store.Get(tprGVK, metav1.NamespaceNone, name)
	if err != nil || !exists {
		return nil, err
	}
	return tpr.(*extensions.ThirdPartyResource), nil
}
