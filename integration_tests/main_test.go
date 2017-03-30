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
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	apiv1 "k8s.io/client-go/pkg/api/v1"
)

func TestWorkflow(t *testing.T) {
	config, err := resources.ConfigFromEnv()
	require.NoError(t, err)

	bundleClient, err := resources.GetBundleTprClient(config, resources.GetBundleScheme())
	require.NoError(t, err)

	clients := dynamic.NewClientPool(config, nil, dynamic.LegacyAPIPathResolverFunc)

	bundleName := "bundle1"
	bundleNamespace := "default"

	var bundleCreated bool
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
	defer func() {
		if bundleCreated {
			t.Logf("Deleting bundle %s", bundleName)
			assert.NoError(t, bundleClient.Delete().
				Namespace(bundleNamespace).
				Resource(smith.BundleResourcePath).
				Name(bundleName).
				Do().
				Error())
			for _, resource := range bundle.Spec.Resources {
				t.Logf("Deleting resource %s", resource.Spec.GetName())
				gv, err := schema.ParseGroupVersion(resource.Spec.GetAPIVersion())
				if !assert.NoError(t, err) {
					continue
				}
				client, err := clients.ClientForGroupVersionKind(gv.WithKind(resource.Spec.GetKind()))
				if !assert.NoError(t, err) {
					continue
				}
				assert.NoError(t, client.Resource(&metav1.APIResource{
					Name:       resources.ResourceKindToPath(resource.Spec.GetKind()),
					Namespaced: true,
					Kind:       resource.Spec.GetKind(),
				}, bundleNamespace).Delete(resource.Spec.GetName(), &metav1.DeleteOptions{}))
			}
		}
	}()

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
		if err := apl.Run(ctx); err != context.Canceled && err != context.DeadlineExceeded {
			assert.NoError(t, err)
		}
	}()

	time.Sleep(1 * time.Second) // Wait until the app starts and creates the Bundle TPR

	t.Log("Creating a new bundle")
	var bundleRes smith.Bundle
	require.NoError(t, bundleClient.Post().
		Namespace(bundleNamespace).
		Resource(smith.BundleResourcePath).
		Body(&bundle).
		Do().
		Into(&bundleRes))

	bundleCreated = true

	for _, resource := range bundle.Spec.Resources {
		func() {
			c, err := clients.ClientForGroupVersionKind(resource.Spec.GroupVersionKind())
			require.NoError(t, err)
			w, err := c.Resource(&metav1.APIResource{
				Name:       resources.ResourceKindToPath(resource.Spec.GetKind()),
				Namespaced: true,
				Kind:       resource.Spec.GetKind(),
			}, bundleNamespace).Watch(metav1.ListOptions{})
			require.NoError(t, err)
			defer w.Stop()
			ctxTimeout, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			for {
				select {
				case <-ctxTimeout.Done():
					t.Fatalf("Timeout waiting for events for resource %s", resource.Name)
				case ev := <-w.ResultChan():
					t.Logf("event %#v", ev)
					if ev.Type != watch.Added || ev.Object.GetObjectKind().GroupVersionKind() != resource.Spec.GroupVersionKind() {
						continue
					}
					obj := ev.Object.(*unstructured.Unstructured)
					if obj.GetName() != resource.Spec.GetName() {
						continue
					}
					t.Logf("received event for resource %q of kind %q", resource.Spec.GetName(), resource.Spec.GetKind())
					assert.Equal(t, map[string]string{
						"configLabel":         "configValue",
						"bundleLabel":         "bundleValue",
						"overlappingLabel":    "overlappingConfigValue",
						smith.BundleNameLabel: bundleName,
					}, obj.GetLabels())
					// TODO uncomment when https://github.com/kubernetes/kubernetes/issues/39816 is fixed
					//a.Equal([]metav1.OwnerReference{
					//	{
					//		APIVersion: smith.BundleResourceVersion,
					//		Kind:       smith.BundleResourceKind,
					//		Name:       bundleName,
					//		UID:        bundleRes.Metadata.UID,
					//	},
					//}, obj.GetOwnerReferences())
					return
				}
			}
		}()
	}
	time.Sleep(500 * time.Millisecond) // Wait a bit to let the server update the status
	require.NoError(t, bundleClient.Get().
		Namespace(bundleNamespace).
		Resource(smith.BundleResourcePath).
		Name(bundleName).
		Do().
		Into(&bundleRes))
	assertCondition(t, &bundleRes, smith.BundleReady, apiv1.ConditionTrue)
	assertCondition(t, &bundleRes, smith.BundleInProgress, apiv1.ConditionFalse)
	assertCondition(t, &bundleRes, smith.BundleError, apiv1.ConditionFalse)
	require.Equal(t, bundle.Spec, bundleRes.Spec, "%#v", bundleRes)
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
