// +build integration_sc

package integration_tests

import (
	"context"
	"testing"

	"github.com/atlassian/smith"
	"github.com/atlassian/smith/pkg/resources"

	sc_v1a1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1alpha1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	api_v1 "k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/rest"
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
					Spec: toUnstructured(t, instance),
				},
				{
					Name:      smith.ResourceName(binding.Name),
					DependsOn: []smith.ResourceName{smith.ResourceName(instance.Name)},
					Spec:      toUnstructured(t, binding),
				},
			},
		},
	}
	setupApp(t, bundle, true, true, testServiceCatalog)
}

func testServiceCatalog(t *testing.T, ctx context.Context, namespace string, bundle *smith.Bundle, config *rest.Config, clientset *kubernetes.Clientset,
	sc smith.SmartClient, bundleClient *rest.RESTClient, bundleCreated *bool, store *resources.Store, args ...interface{}) {

	assertBundleTimeout(t, ctx, store, namespace, bundle, "")
}
