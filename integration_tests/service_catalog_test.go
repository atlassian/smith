// +build integration_sc

package integration_tests

import (
	"context"
	"testing"
	"time"

	"github.com/atlassian/smith"
	"github.com/atlassian/smith/pkg/resources"

	scv1alpha1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1alpha1"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	apiv1 "k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/rest"
)

func TestServiceCatalog(t *testing.T) {
	instance := &scv1alpha1.Instance{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Instance",
			APIVersion: scv1alpha1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "instance1",
		},
		Spec: scv1alpha1.InstanceSpec{
			ServiceClassName: "user-provided-service",
			PlanName:         "default",
		},
	}
	binding := &scv1alpha1.Binding{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Binding",
			APIVersion: scv1alpha1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "binding1",
		},
		Spec: scv1alpha1.BindingSpec{
			InstanceRef: apiv1.LocalObjectReference{
				Name: instance.Name,
			},
			SecretName: "secret1",
		},
	}
	bundle := &smith.Bundle{
		TypeMeta: metav1.TypeMeta{
			Kind:       smith.BundleResourceKind,
			APIVersion: smith.BundleResourceGroupVersion,
		},
		Metadata: metav1.ObjectMeta{
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
	clients, scDynamic dynamic.ClientPool, bundleClient *rest.RESTClient, bundleCreated *bool, store *resources.Store, args ...interface{}) {

	ctxTimeout, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	obj, err := store.AwaitObjectCondition(ctxTimeout, smith.BundleGVK, namespace, bundle.Metadata.Name, isBundleReady)
	require.NoError(t, err)
	bundleRes := obj.(*smith.Bundle)

	assertCondition(t, bundleRes, smith.BundleReady, smith.ConditionTrue)
	assertCondition(t, bundleRes, smith.BundleInProgress, smith.ConditionFalse)
	assertCondition(t, bundleRes, smith.BundleError, smith.ConditionFalse)
}
