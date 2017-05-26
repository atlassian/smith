// +build integration

package integration_tests

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/atlassian/smith"
	"github.com/atlassian/smith/examples/tprattribute"
	"github.com/atlassian/smith/pkg/resources"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	api_v1 "k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/rest"
)

func TestUpdate(t *testing.T) {
	existingConfigMap := &api_v1.ConfigMap{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: "v1",
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "config2",
			Labels: map[string]string{
				"labelx": "labelxValue",
			},
		},
		Data: map[string]string{
			"a": "b",
		},
	}
	bundleConfigMap := &api_v1.ConfigMap{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: "v1",
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: existingConfigMap.Name,
			Labels: map[string]string{
				"configLabel":         "configValue",
				"overlappingLabel":    "overlappingConfigValue",
				smith.BundleNameLabel: "configLabel123",
			},
		},
		Data: map[string]string{
			"x": "y",
		},
	}
	existingSleeper := &tprattribute.Sleeper{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       tprattribute.SleeperResourceKind,
			APIVersion: tprattribute.SleeperResourceGroupVersion,
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "sleeper2",
			Labels: map[string]string{
				"labelx": "labelxValue",
			},
		},
		Spec: tprattribute.SleeperSpec{
			SleepFor:      10, // seconds,
			WakeupMessage: "Hello there!",
		},
	}
	bundleSleeper := &tprattribute.Sleeper{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       tprattribute.SleeperResourceKind,
			APIVersion: tprattribute.SleeperResourceGroupVersion,
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: existingSleeper.Name,
			Labels: map[string]string{
				"configLabel":         "configValue",
				"overlappingLabel":    "overlappingConfigValue",
				smith.BundleNameLabel: "configLabel123",
			},
		},
		Spec: tprattribute.SleeperSpec{
			SleepFor:      1, // seconds,
			WakeupMessage: "Hello, martians!",
		},
	}
	bundle := &smith.Bundle{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       smith.BundleResourceKind,
			APIVersion: smith.BundleResourceGroupVersion,
		},
		ObjectMeta: meta_v1.ObjectMeta{
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
					Name: smith.ResourceName(bundleConfigMap.Name),
					Spec: toUnstructured(t, bundleConfigMap),
				},
				{
					Name: smith.ResourceName(bundleSleeper.Name),
					Spec: toUnstructured(t, bundleSleeper),
				},
			},
		},
	}
	setupApp(t, bundle, false, false, testUpdate, existingConfigMap, bundleConfigMap, existingSleeper, bundleSleeper)
}

func testUpdate(t *testing.T, ctx context.Context, namespace string, bundle *smith.Bundle, config *rest.Config, clientset *kubernetes.Clientset,
	clients, scDynamic dynamic.ClientPool, bundleClient *rest.RESTClient, bundleCreated *bool, store *resources.Store, args ...interface{}) {

	var wg sync.WaitGroup
	defer wg.Wait()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	wg.Add(1)
	go func() {
		defer wg.Done()
		apl := tprattribute.App{
			RestConfig: config,
		}
		if e := apl.Run(ctx); e != context.Canceled && e != context.DeadlineExceeded {
			assert.NoError(t, e)
		}
	}()

	existingConfigMap := args[0].(*api_v1.ConfigMap)
	bundleConfigMap := args[1].(*api_v1.ConfigMap)
	existingSleeper := args[2].(*tprattribute.Sleeper)
	bundleSleeper := args[3].(*tprattribute.Sleeper)

	cmClient := clientset.CoreV1().ConfigMaps(namespace)
	_, err := cmClient.Create(existingConfigMap)
	require.NoError(t, err)
	defer func() {
		t.Logf("Deleting resource %q", existingConfigMap.Name)
		e := cmClient.Delete(existingConfigMap.Name, nil)
		if !kerrors.IsNotFound(e) { // May have been cleanup by cleanupBundle
			assert.NoError(t, e)
		}
	}()

	sClient, err := tprattribute.GetSleeperTprClient(config, sleeperScheme())
	require.NoError(t, err)

	time.Sleep(500 * time.Millisecond) // Wait until apps start and create the Sleeper and Bundle TPRs

	createObject(t, existingSleeper, namespace, tprattribute.SleeperResourcePath, sClient)
	defer func() {
		t.Logf("Maybe deleting resource %q", existingSleeper.Name)
		e := sClient.Delete().
			Namespace(namespace).
			Resource(tprattribute.SleeperResourcePath).
			Name(existingSleeper.Name).
			Do().
			Error()
		if !kerrors.IsNotFound(e) { // May have been cleanup by cleanupBundle
			assert.NoError(t, e)
		}
	}()

	createObject(t, bundle, namespace, smith.BundleResourcePath, bundleClient)
	created := true
	defer cleanupBundle(t, namespace, bundleClient, clients, scDynamic, &created, bundle)

	ctxTimeout, cancel := context.WithTimeout(ctx, time.Duration(bundleSleeper.Spec.SleepFor+existingSleeper.Spec.SleepFor+2)*time.Second)
	defer cancel()

	bundleRes := assertBundle(t, ctxTimeout, store, namespace, bundle, "")

	cfMap, err := cmClient.Get(bundleConfigMap.Name, meta_v1.GetOptions{})
	if assert.NoError(t, err) {
		assert.Equal(t, map[string]string{
			"configLabel":         "configValue",
			"bundleLabel":         "bundleValue",
			"overlappingLabel":    "overlappingConfigValue",
			smith.BundleNameLabel: bundle.Name,
		}, cfMap.Labels)
		assert.Equal(t, bundleConfigMap.Data, cfMap.Data)
	}

	var sleeperObj tprattribute.Sleeper
	err = sClient.Get().
		Namespace(namespace).
		Resource(tprattribute.SleeperResourcePath).
		Name(bundleSleeper.Name).
		Do().
		Into(&sleeperObj)
	if assert.NoError(t, err) {
		assert.Equal(t, map[string]string{
			"configLabel":         "configValue",
			"bundleLabel":         "bundleValue",
			"overlappingLabel":    "overlappingConfigValue",
			smith.BundleNameLabel: bundle.Name,
		}, sleeperObj.Labels)
		assert.Equal(t, tprattribute.Awake, sleeperObj.Status.State)
		assert.Equal(t, bundleSleeper.Spec, sleeperObj.Spec)
	}
	emptyBundle := *bundle
	emptyBundle.Spec.Resources = []smith.Resource{}
	require.NoError(t, bundleClient.Put().
		Namespace(namespace).
		Resource(smith.BundleResourcePath).
		Name(bundle.Name).
		Body(&emptyBundle).
		Do().
		Error())

	assertBundleTimeout(t, ctx, store, namespace, &emptyBundle, bundleRes.ResourceVersion)

	cfMap, err = cmClient.Get(bundleConfigMap.Name, meta_v1.GetOptions{})
	if err == nil {
		assert.NotNil(t, cfMap.DeletionTimestamp) // Still in api but marked for deletion
	} else {
		assert.True(t, kerrors.IsNotFound(err)) // Has been removed from api already
	}
	err = sClient.Get().
		Namespace(namespace).
		Resource(tprattribute.SleeperResourcePath).
		Name(bundleSleeper.Name).
		Do().
		Into(&sleeperObj)
	if err == nil {
		assert.NotNil(t, sleeperObj.DeletionTimestamp) // Still in api but marked for deletion
	} else {
		assert.True(t, kerrors.IsNotFound(err)) // Has been removed from api already
	}
}
