package app

import (
	"context"

	"github.com/atlassian/smith"

	sc_v1a1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1alpha1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	api_v1 "k8s.io/client-go/pkg/api/v1"
	apps_v1b1 "k8s.io/client-go/pkg/apis/apps/v1beta1"
	ext_v1b1 "k8s.io/client-go/pkg/apis/extensions/v1beta1"
	settings_v1a1 "k8s.io/client-go/pkg/apis/settings/v1alpha1"
)

var (
	tprGVK = ext_v1b1.SchemeGroupVersion.WithKind("ThirdPartyResource")
)

type Processor interface {
	Rebuild(context.Context, *smith.Bundle) error
}

func FullScheme(serviceCatalog bool) (*runtime.Scheme, error) {
	scheme := runtime.NewScheme()
	smith.AddToScheme(scheme)
	var sb runtime.SchemeBuilder
	sb.Register(ext_v1b1.SchemeBuilder...)
	sb.Register(api_v1.SchemeBuilder...)
	sb.Register(apps_v1b1.SchemeBuilder...)
	sb.Register(settings_v1a1.SchemeBuilder...)
	if serviceCatalog {
		sb.Register(sc_v1a1.SchemeBuilder...)
	} else {
		scheme.AddUnversionedTypes(api_v1.SchemeGroupVersion, &meta_v1.Status{})
	}
	if err := sb.AddToScheme(scheme); err != nil {
		return nil, err
	}
	return scheme, nil
}
