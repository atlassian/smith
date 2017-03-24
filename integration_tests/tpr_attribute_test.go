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
)

func TestTprAttribute(t *testing.T) {
	config, err := resources.ConfigFromEnv()
	require.NoError(t, err)

	bundleClient, err := resources.GetBundleTprClient(config, resources.GetBundleScheme())
	require.NoError(t, err)

	sClient, err := tprattribute.GetSleeperTprClient(config, tprattribute.GetSleeperScheme())
	require.NoError(t, err)

	bundleName := "bundle-attribute"
	bundleNamespace := "default"

	var bundleCreated bool
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
	defer func() {
		if bundleCreated {
			t.Logf("Deleting bundle %s", bundleName)
			assert.NoError(t, bundleClient.Delete().
				Namespace(bundleNamespace).
				Resource(smith.BundleResourcePath).
				Name(bundleName).
				Do().
				Error())
			t.Logf("Deleting resource %s", sleeper.Metadata.Name)
			assert.NoError(t, sClient.Delete().
				Namespace(bundleNamespace).
				Resource(tprattribute.SleeperResourcePath).
				Name(sleeper.Metadata.Name).
				Do().
				Error())
		}
	}()

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
		if err := apl.Run(ctx); err != context.Canceled && err != context.DeadlineExceeded {
			assert.NoError(t, err)
		}
	}()
	go func() {
		defer wg.Done()

		apl := tprattribute.App{
			RestConfig: config,
		}
		if err := apl.Run(ctx); err != context.Canceled && err != context.DeadlineExceeded {
			assert.NoError(t, err)
		}
	}()

	time.Sleep(5 * time.Second) // Wait until apps start and creates the Bundle TPR and Sleeper TPR

	t.Log("Creating a new bundle")
	require.NoError(t, bundleClient.Post().
		Namespace(bundleNamespace).
		Resource(smith.BundleResourcePath).
		Body(&bundle).
		Do().
		Error())

	bundleCreated = true

	func() {
		w, err := sClient.Get().
			Namespace(bundleNamespace).
			Prefix("watch").
			Resource(tprattribute.SleeperResourcePath).
			Watch()
		require.NoError(t, err)
		defer w.Stop()
		ctxTimeout, cancel := context.WithTimeout(ctx, time.Duration(sleeper.Spec.SleepFor+1)*time.Second)
		defer cancel()
		for {
			select {
			case <-ctxTimeout.Done():
				t.Fatalf("Timeout waiting for events for resource %q", sleeper.Metadata.Name)
			case ev := <-w.ResultChan():
				t.Logf("event %#v", ev)
				obj := ev.Object.(*tprattribute.Sleeper)
				if obj.Metadata.Name != sleeper.Metadata.Name {
					continue
				}
				t.Logf("received event with status.state == %q for resource %q of kind %q", obj.Status.State, sleeper.Metadata.Name, sleeper.Kind)
				assert.EqualValues(t, map[string]string{
					smith.BundleNameLabel: bundleName,
				}, obj.Metadata.Labels)
				if obj.Status.State == tprattribute.AWAKE {
					return
				}
			}
		}
	}()
	time.Sleep(500 * time.Millisecond) // Wait a bit to let the server update the status
	var bundleRes smith.Bundle
	require.NoError(t, bundleClient.Get().
		Namespace(bundleNamespace).
		Resource(smith.BundleResourcePath).
		Name(bundleName).
		Do().
		Into(&bundleRes))
	require.Equal(t, smith.READY, bundleRes.Status.State)
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
