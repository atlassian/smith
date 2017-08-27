package sc

import (
	"context"
	"testing"

	"github.com/atlassian/smith/it"
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"

	sc_v1a1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1alpha1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	api_v1 "k8s.io/client-go/pkg/api/v1"
)

func TestServiceCatalog(t *testing.T) {
	instance := &sc_v1a1.Instance{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "Instance",
			APIVersion: sc_v1a1.SchemeGroupVersion.String(),
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "instance1",
		},
		Spec: sc_v1a1.InstanceSpec{
			ServiceClassName: "user-provided-service",
			PlanName:         "default",
		},
	}
	binding := &sc_v1a1.Binding{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "Binding",
			APIVersion: sc_v1a1.SchemeGroupVersion.String(),
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "binding1",
		},
		Spec: sc_v1a1.BindingSpec{
			InstanceRef: api_v1.LocalObjectReference{
				Name: instance.Name,
			},
			SecretName: "secret1",
		},
	}
	bundle := &smith_v1.Bundle{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       smith_v1.BundleResourceKind,
			APIVersion: smith_v1.BundleResourceGroupVersion,
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "bundle-cs",
		},
		Spec: smith_v1.BundleSpec{
			Resources: []smith_v1.Resource{
				{
					Name: smith_v1.ResourceName(instance.Name),
					Spec: instance,
				},
				{
					Name:      smith_v1.ResourceName(binding.Name),
					DependsOn: []smith_v1.ResourceName{smith_v1.ResourceName(instance.Name)},
					Spec:      binding,
				},
			},
		},
	}
	it.SetupApp(t, bundle, true, true, testServiceCatalog)
}

func testServiceCatalog(t *testing.T, ctx context.Context, cfg *it.ItConfig, args ...interface{}) {
	cfg.AssertBundleTimeout(ctx, cfg.Bundle, "")
}
