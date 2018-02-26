package apitypes

import (
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"

	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	apps_v1 "k8s.io/api/apps/v1"
	core_v1 "k8s.io/api/core/v1"
	ext_v1b1 "k8s.io/api/extensions/v1beta1"
	settings_v1a1 "k8s.io/api/settings/v1alpha1"
	apiext_v1b1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func FullScheme(serviceCatalog bool) (*runtime.Scheme, error) {
	scheme := runtime.NewScheme()
	var sb runtime.SchemeBuilder
	sb.Register(smith_v1.SchemeBuilder...)
	sb.Register(ext_v1b1.SchemeBuilder...)
	sb.Register(core_v1.SchemeBuilder...)
	sb.Register(apps_v1.SchemeBuilder...)
	sb.Register(settings_v1a1.SchemeBuilder...)
	sb.Register(apiext_v1b1.SchemeBuilder...)
	if serviceCatalog {
		sb.Register(sc_v1b1.SchemeBuilder...)
	}
	scheme.AddUnversionedTypes(core_v1.SchemeGroupVersion, &meta_v1.Status{})
	if err := sb.AddToScheme(scheme); err != nil {
		return nil, err
	}
	return scheme, nil
}
