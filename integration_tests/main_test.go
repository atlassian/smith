// +build integration

package integration_tests

import (
	"context"
	"testing"

	"github.com/atlassian/smith"
	"github.com/atlassian/smith/pkg/resources"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	apiv1 "k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/rest"
)

func TestWorkflow(t *testing.T) {
	c1 := apiv1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "config1",
			Labels: map[string]string{
				"configLabel":         "configValue",
				"overlappingLabel":    "overlappingConfigValue",
				smith.BundleNameLabel: "configLabel123",
			},
		},
		Data: map[string]string{
			"a": "b",
		},
	}
	s1 := apiv1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "secret1",
		},
		StringData: map[string]string{
			"a": "b",
		},
	}
	bundle := &smith.Bundle{
		TypeMeta: metav1.TypeMeta{
			Kind:       smith.BundleResourceKind,
			APIVersion: smith.BundleResourceGroupVersion,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "bundle1",
			Labels: map[string]string{
				"bundleLabel":         "bundleValue",
				"overlappingLabel":    "overlappingBundleValue",
				smith.BundleNameLabel: "bundleLabel123",
			},
		},
		Spec: smith.BundleSpec{
			Resources: []smith.Resource{
				{
					Name:      "config1res",
					DependsOn: []smith.ResourceName{"secret2res"},
					Spec:      toUnstructured(t, &c1),
				},
				{
					Name: "secret2res",
					Spec: toUnstructured(t, &s1),
				},
			},
		},
	}
	setupApp(t, bundle, false, true, testWorkflow)
}

func testWorkflow(t *testing.T, ctx context.Context, namespace string, bundle *smith.Bundle, config *rest.Config, clientset *kubernetes.Clientset,
	clients, scDynamic dynamic.ClientPool, bundleClient *rest.RESTClient, bundleCreated *bool, store *resources.Store, args ...interface{}) {

	bundleRes := assertBundleTimeout(t, ctx, store, namespace, bundle, "")

	cfMap, err := clientset.CoreV1().ConfigMaps(namespace).Get("config1", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, map[string]string{
		"configLabel":         "configValue",
		"bundleLabel":         "bundleValue",
		"overlappingLabel":    "overlappingConfigValue",
		smith.BundleNameLabel: bundleRes.Name,
	}, cfMap.GetLabels())

	secret, err := clientset.CoreV1().Secrets(namespace).Get("secret1", metav1.GetOptions{})
	require.NoError(t, err)
	trueRef := true
	assert.Equal(t, []metav1.OwnerReference{
		{
			APIVersion:         smith.BundleResourceGroupVersion,
			Kind:               smith.BundleResourceKind,
			Name:               bundleRes.Name,
			UID:                bundleRes.UID,
			BlockOwnerDeletion: &trueRef,
		},
		{
			APIVersion:         "v1",
			Kind:               "Secret",
			Name:               secret.Name,
			UID:                secret.UID,
			BlockOwnerDeletion: &trueRef,
		},
	}, cfMap.GetOwnerReferences())
}
