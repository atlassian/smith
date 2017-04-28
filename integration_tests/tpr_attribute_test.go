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

func TestTprAttribute(t *testing.T) {
	sleeper, sleeperU := bundleAttrResources(t)
	bundle := &smith.Bundle{
		TypeMeta: metav1.TypeMeta{
			Kind:       smith.BundleResourceKind,
			APIVersion: smith.BundleResourceGroupVersion,
		},
		Metadata: metav1.ObjectMeta{
			Name:      "bundle-attribute",
			Namespace: "default",
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
	setupApp(t, bundle, false, testTprAttribute, sleeper)
}

func testTprAttribute(t *testing.T, ctx context.Context, bundle *smith.Bundle, config *rest.Config, clientset *kubernetes.Clientset,
	clients dynamic.ClientPool, bundleClient *rest.RESTClient, store *resources.Store, args ...interface{}) {

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

	bundleInf := bundleInformer(bundleClient)

	store.AddInformer(smith.BundleGVK, bundleInf)

	ctxTimeout, cancel := context.WithTimeout(ctx, time.Duration(sleeper.Spec.SleepFor+3)*time.Second)
	defer cancel()
	go bundleInf.Run(ctxTimeout.Done())

	obj, err := store.AwaitObjectCondition(ctxTimeout, smith.BundleGVK, bundle.Metadata.Namespace, bundle.Metadata.Name, isBundleReady)
	require.NoError(t, err)
	bundleRes := obj.(*smith.Bundle)

	assertCondition(t, bundleRes, smith.BundleReady, apiv1.ConditionTrue)
	assertCondition(t, bundleRes, smith.BundleInProgress, apiv1.ConditionFalse)
	assertCondition(t, bundleRes, smith.BundleError, apiv1.ConditionFalse)

	var sleeperObj tprattribute.Sleeper
	require.NoError(t, sClient.Get().
		Namespace(bundle.Metadata.Namespace).
		Resource(tprattribute.SleeperResourcePath).
		Name(sleeper.Metadata.Name).
		Do().
		Into(&sleeperObj))

	assert.Equal(t, map[string]string{
		smith.BundleNameLabel: bundle.Metadata.Name,
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
			SleepFor:      1, // seconds,
			WakeupMessage: "Hello, Infravators!",
		},
	}
	data, err := json.Marshal(c)
	require.NoError(t, err)

	u := &unstructured.Unstructured{}
	require.NoError(t, u.UnmarshalJSON(data))
	return c, u
}
