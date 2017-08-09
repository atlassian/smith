// +build integration_sc

package integration_tests

import (
	"context"
	"testing"

	"github.com/atlassian/smith"

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
	bundle := &smith.Bundle{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       smith.BundleResourceKind,
			APIVersion: smith.BundleResourceGroupVersion,
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "bundle-cs",
		},
		Spec: smith.BundleSpec{
			Resources: []smith.Resource{
				{
					Name: smith.ResourceName(instance.Name),
					Spec: instance,
				},
				{
					Name:      smith.ResourceName(binding.Name),
					DependsOn: []smith.ResourceName{smith.ResourceName(instance.Name)},
					Spec:      binding,
				},
			},
		},
	}
	setupApp(t, bundle, true, true, testServiceCatalog)
}

func testServiceCatalog(t *testing.T, ctx context.Context, cfg *itConfig, args ...interface{}) {
	cfg.assertBundleTimeout(ctx, cfg.bundle, "")
}
