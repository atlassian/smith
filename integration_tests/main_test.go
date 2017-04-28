// +build integration

package integration_tests

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/atlassian/smith"
	"github.com/atlassian/smith/pkg/resources"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
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
		Metadata: metav1.ObjectMeta{
			Name:      "bundle1",
			Namespace: "default",
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
	setupApp(t, bundle, false, testWorkflow)
}

func testWorkflow(t *testing.T, ctx context.Context, bundle *smith.Bundle, config *rest.Config, clientset *kubernetes.Clientset,
	clients dynamic.ClientPool, bundleClient *rest.RESTClient, store *resources.Store, args ...interface{}) {

	bundleInf := bundleInformer(bundleClient)

	store.AddInformer(smith.BundleGVK, bundleInf)

	ctxTimeout, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	go bundleInf.Run(ctxTimeout.Done())

	obj, err := store.AwaitObjectCondition(ctxTimeout, smith.BundleGVK, bundle.Metadata.Namespace, bundle.Metadata.Name, isBundleReady)
	require.NoError(t, err)
	bundleRes := obj.(*smith.Bundle)

	assertCondition(t, bundleRes, smith.BundleReady, apiv1.ConditionTrue)
	assertCondition(t, bundleRes, smith.BundleInProgress, apiv1.ConditionFalse)
	assertCondition(t, bundleRes, smith.BundleError, apiv1.ConditionFalse)
	assert.Equal(t, bundle.Spec, bundleRes.Spec, "%#v", bundleRes)

	cfMap, err := clientset.CoreV1().ConfigMaps(bundle.Metadata.Namespace).Get("config1", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, map[string]string{
		"configLabel":         "configValue",
		"bundleLabel":         "bundleValue",
		"overlappingLabel":    "overlappingConfigValue",
		smith.BundleNameLabel: bundle.Metadata.Name,
	}, cfMap.GetLabels())
	// TODO uncomment when https://github.com/kubernetes/kubernetes/issues/39816 is fixed
	//assert.Equal(t, []metav1.OwnerReference{
	//	{
	//		APIVersion: smith.BundleResourceVersion,
	//		Kind:       smith.BundleResourceKind,
	//		Name:       bundleName,
	//		UID:        bundleRes.Metadata.UID,
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
	data, err := json.Marshal(&c)
	require.NoError(t, err)

	r1 := unstructured.Unstructured{}
	require.NoError(t, r1.UnmarshalJSON(data))
	return []smith.Resource{
		{
			Name: "resource1",
			Spec: r1,
		},
	}
}
