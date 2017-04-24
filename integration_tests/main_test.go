// +build integration

package integration_tests

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/atlassian/smith"
	"github.com/atlassian/smith/pkg/app"
	"github.com/atlassian/smith/pkg/resources"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	apiv1 "k8s.io/client-go/pkg/api/v1"
)

func TestWorkflow(t *testing.T) {
	config, err := resources.ConfigFromEnv()
	require.NoError(t, err)

	clientset, err := kubernetes.NewForConfig(config)
	require.NoError(t, err)

	scheme := resources.BundleScheme()
	bundleClient, err := resources.GetBundleTprClient(config, scheme)
	require.NoError(t, err)

	clients := dynamic.NewClientPool(config, nil, dynamic.LegacyAPIPathResolverFunc)

	bundleName := "bundle1"
	bundleNamespace := "default"

	bundle := smith.Bundle{
		TypeMeta: metav1.TypeMeta{
			Kind:       smith.BundleResourceKind,
			APIVersion: smith.BundleResourceGroupVersion,
		},
		Metadata: metav1.ObjectMeta{
			Name: bundleName,
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
	err = bundleClient.Delete().
		Namespace(bundleNamespace).
		Resource(smith.BundleResourcePath).
		Name(bundleName).
		Do().
		Error()
	if err == nil {
		t.Log("Bundle deleted")
	} else if !errors.IsNotFound(err) {
		require.NoError(t, err)
	}
	var bundleCreated bool
	defer cleanupBundle(t, bundleClient, clients, &bundleCreated, &bundle, bundleNamespace)

	var wg sync.WaitGroup
	defer wg.Wait()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	wg.Add(1)
	go func() {
		defer wg.Done()
		apl := app.App{
			RestConfig: config,
		}
		if e := apl.Run(ctx); e != context.Canceled && e != context.DeadlineExceeded {
			assert.NoError(t, e)
		}
	}()

	time.Sleep(1 * time.Second) // Wait until the app starts and creates the Bundle TPR

	t.Log("Creating a new bundle")
	require.NoError(t, bundleClient.Post().
		Namespace(bundleNamespace).
		Resource(smith.BundleResourcePath).
		Body(&bundle).
		Do().
		Error())

	bundleCreated = true

	store := resources.NewStore(scheme.DeepCopy)
	bundleInf := bundleInformer(bundleClient)

	var wgStore sync.WaitGroup
	defer wgStore.Wait() // await store termination

	ctxStore, cancelStore := context.WithCancel(context.Background())
	defer cancelStore() // signal store to stop
	wgStore.Add(1)
	go store.Run(ctxStore, wgStore.Done)

	store.AddInformer(smith.BundleGVK, bundleInf)

	ctxTimeout, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	go bundleInf.Run(ctxTimeout.Done())

	obj, err := store.AwaitObjectCondition(ctxTimeout, smith.BundleGVK, bundleNamespace, bundleName, awaitBundleReady)
	require.NoError(t, err)
	bundleRes := obj.(*smith.Bundle)

	assertCondition(t, bundleRes, smith.BundleReady, apiv1.ConditionTrue)
	assertCondition(t, bundleRes, smith.BundleInProgress, apiv1.ConditionFalse)
	assertCondition(t, bundleRes, smith.BundleError, apiv1.ConditionFalse)
	assert.Equal(t, bundle.Spec, bundleRes.Spec, "%#v", bundleRes)

	cfMap, err := clientset.CoreV1().ConfigMaps(bundleNamespace).Get("config1", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, map[string]string{
		"configLabel":         "configValue",
		"bundleLabel":         "bundleValue",
		"overlappingLabel":    "overlappingConfigValue",
		smith.BundleNameLabel: bundleName,
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
