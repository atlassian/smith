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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func TestTprAttribute(t *testing.T) {
	sleeper, sleeperU := bundleAttrResources(t)
	bundle := &smith.Bundle{
		TypeMeta: metav1.TypeMeta{
			Kind:       smith.BundleResourceKind,
			APIVersion: smith.BundleResourceGroupVersion,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "bundle-attribute",
		},
		Spec: smith.BundleSpec{
			Resources: []smith.Resource{
				{
					Name: smith.ResourceName(sleeper.Name),
					Spec: sleeperU,
				},
			},
		},
	}
	setupApp(t, bundle, false, true, testTprAttribute, sleeper)
}

func testTprAttribute(t *testing.T, ctx context.Context, namespace string, bundle *smith.Bundle, config *rest.Config, clientset *kubernetes.Clientset,
	clients, scDynamic dynamic.ClientPool, bundleClient *rest.RESTClient, bundleCreated *bool, store *resources.Store, args ...interface{}) {

	sleeper := args[0].(*tprattribute.Sleeper)
	sClient, err := tprattribute.GetSleeperTprClient(config, sleeperScheme())
	require.NoError(t, err)

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

	ctxTimeout, cancel := context.WithTimeout(ctx, time.Duration(sleeper.Spec.SleepFor+3)*time.Second)
	defer cancel()

	assertBundle(t, ctxTimeout, store, namespace, bundle, "")

	var sleeperObj tprattribute.Sleeper
	require.NoError(t, sClient.Get().
		Namespace(namespace).
		Resource(tprattribute.SleeperResourcePath).
		Name(sleeper.Name).
		Do().
		Into(&sleeperObj))

	assert.Equal(t, map[string]string{
		smith.BundleNameLabel: bundle.Name,
	}, sleeperObj.Labels)
	assert.Equal(t, tprattribute.Awake, sleeperObj.Status.State)
}

func bundleAttrResources(t *testing.T) (*tprattribute.Sleeper, unstructured.Unstructured) {
	c := &tprattribute.Sleeper{
		TypeMeta: metav1.TypeMeta{
			Kind:       tprattribute.SleeperResourceKind,
			APIVersion: tprattribute.SleeperResourceGroupVersion,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "sleeper1",
		},
		Spec: tprattribute.SleeperSpec{
			SleepFor:      1, // seconds,
			WakeupMessage: "Hello, Infravators!",
		},
	}
	return c, toUnstructured(t, c)
}
