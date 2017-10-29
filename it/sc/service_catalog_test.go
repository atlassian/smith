package sc

import (
	"context"
	"testing"

	"github.com/atlassian/smith/it"
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"

	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestServiceCatalog(t *testing.T) {
	t.Parallel()
	instance := &sc_v1b1.ServiceInstance{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "ServiceInstance",
			APIVersion: sc_v1b1.SchemeGroupVersion.String(),
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "instance1",
		},
		Spec: sc_v1b1.ServiceInstanceSpec{
			PlanReference: sc_v1b1.PlanReference{
				ClusterServiceClassExternalName: "user-provided-service",
				ClusterServicePlanExternalName:  "default",
			},
		},
	}
	binding := &sc_v1b1.ServiceBinding{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "ServiceBinding",
			APIVersion: sc_v1b1.SchemeGroupVersion.String(),
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "binding1",
		},
		Spec: sc_v1b1.ServiceBindingSpec{
			ServiceInstanceRef: sc_v1b1.LocalObjectReference{
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
