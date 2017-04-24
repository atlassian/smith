// +build integration

package integration_tests

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/atlassian/smith"
	"github.com/atlassian/smith/examples/tprattribute"
	"github.com/atlassian/smith/pkg/app"
	"github.com/atlassian/smith/pkg/resources"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
	apiv1 "k8s.io/client-go/pkg/api/v1"
)

func TestTprAttribute(t *testing.T) {
	config, err := resources.ConfigFromEnv()
	require.NoError(t, err)

	scheme := resources.BundleScheme()
	bundleClient, err := resources.GetBundleTprClient(config, scheme)
	require.NoError(t, err)

	sClient, err := tprattribute.GetSleeperTprClient(config, sleeperScheme())
	require.NoError(t, err)

	clients := dynamic.NewClientPool(config, nil, dynamic.LegacyAPIPathResolverFunc)

	bundleName := "bundle-attribute"
	bundleNamespace := "default"

	sleeper, sleeperU := bundleAttrResources(t)
	bundle := smith.Bundle{
		TypeMeta: metav1.TypeMeta{
			Kind:       smith.BundleResourceKind,
			APIVersion: smith.BundleResourceGroupVersion,
		},
		Metadata: metav1.ObjectMeta{
			Name: bundleName,
		},
		Spec: smith.BundleSpec{
			Resources: []smith.Resource{
				{
					Name: smith.ResourceName(sleeper.Metadata.Name),
					Spec: *sleeperU,
				},
			},
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

	wg.Add(2)
	go func() {
		defer wg.Done()

		apl := app.App{
			RestConfig: config,
		}
		if e := apl.Run(ctx); e != context.Canceled && e != context.DeadlineExceeded {
			assert.NoError(t, e)
		}
	}()
	go func() {
		defer wg.Done()

		apl := tprattribute.App{
			RestConfig: config,
		}
		if e := apl.Run(ctx); e != context.Canceled && e != context.DeadlineExceeded {
			assert.NoError(t, e)
		}
	}()

	time.Sleep(5 * time.Second) // Wait until apps start and create the Bundle TPR and Sleeper TPR

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

	ctxTimeout, cancel := context.WithTimeout(ctx, time.Duration(sleeper.Spec.SleepFor+1)*time.Second)
	defer cancel()
	go bundleInf.Run(ctxTimeout.Done())

	obj, err := store.AwaitObjectCondition(ctxTimeout, smith.BundleGVK, bundleNamespace, bundleName, awaitBundleReady)
	require.NoError(t, err)
	bundleRes := obj.(*smith.Bundle)

	assertCondition(t, bundleRes, smith.BundleReady, apiv1.ConditionTrue)
	assertCondition(t, bundleRes, smith.BundleInProgress, apiv1.ConditionFalse)
	assertCondition(t, bundleRes, smith.BundleError, apiv1.ConditionFalse)

	var sleeperObj tprattribute.Sleeper
	require.NoError(t, sClient.Get().
		Namespace(bundleNamespace).
		Resource(tprattribute.SleeperResourcePath).
		Name(sleeper.Metadata.Name).
		Do().
		Into(&sleeperObj))

	assert.Equal(t, map[string]string{
		smith.BundleNameLabel: bundleName,
	}, sleeperObj.Metadata.Labels)
	assert.Equal(t, tprattribute.Awake, sleeperObj.Status.State)
}

func bundleAttrResources(t *testing.T) (*tprattribute.Sleeper, *unstructured.Unstructured) {
	c := &tprattribute.Sleeper{
		TypeMeta: metav1.TypeMeta{
			Kind:       tprattribute.SleeperResourceKind,
			APIVersion: tprattribute.SleeperResourceGroupVersion,
		},
		Metadata: metav1.ObjectMeta{
			Name: "sleeper1",
		},
		Spec: tprattribute.SleeperSpec{
			SleepFor:      3, // seconds,
			WakeupMessage: "Hello, Infravators!",
		},
	}
	data, err := json.Marshal(c)
	require.NoError(t, err)

	u := &unstructured.Unstructured{}
	require.NoError(t, u.UnmarshalJSON(data))
	return c, u
}
