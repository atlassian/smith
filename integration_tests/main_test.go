// +build integration

package integration_tests

import (
	"context"
	"testing"
	"time"

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
			Resources: bundleResources(t),
		},
	}
	setupApp(t, bundle, false, true, testWorkflow)
}

func testWorkflow(t *testing.T, ctx context.Context, namespace string, bundle *smith.Bundle, config *rest.Config, clientset *kubernetes.Clientset,
	clients, scDynamic dynamic.ClientPool, bundleClient *rest.RESTClient, bundleCreated *bool, store *resources.Store, args ...interface{}) {

	ctxTimeout, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	obj, err := store.AwaitObjectCondition(ctxTimeout, smith.BundleGVK, namespace, bundle.Name, isBundleReady)
	require.NoError(t, err)
	bundleRes := obj.(*smith.Bundle)

	assertCondition(t, bundleRes, smith.BundleReady, smith.ConditionTrue)
	assertCondition(t, bundleRes, smith.BundleInProgress, smith.ConditionFalse)
	assertCondition(t, bundleRes, smith.BundleError, smith.ConditionFalse)
	assert.Equal(t, bundle.Spec, bundleRes.Spec, "%#v", bundleRes)

	cfMap, err := clientset.CoreV1().ConfigMaps(namespace).Get("config1", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, map[string]string{
		"configLabel":         "configValue",
		"bundleLabel":         "bundleValue",
		"overlappingLabel":    "overlappingConfigValue",
		smith.BundleNameLabel: bundle.Name,
	}, cfMap.GetLabels())
	// TODO uncomment when https://github.com/kubernetes/kubernetes/issues/39816 is fixed
	//assert.Equal(t, []metav1.OwnerReference{
	//	{
	//		APIVersion: smith.BundleResourceVersion,
	//		Kind:       smith.BundleResourceKind,
	//		Name:       bundleName,
	//		UID:        bundleRes.UID,
	//	},
	//}, cfMap.GetOwnerReferences())
}

func bundleResources(t *testing.T) []smith.Resource {
	c := apiv1.ConfigMap{
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
	return []smith.Resource{
		{
			Name: "resource1",
			Spec: toUnstructured(t, &c),
		},
	}
}
