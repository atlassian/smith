// +build integration

package integration_tests

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/atlassian/smith"
	"github.com/atlassian/smith/examples/tprattribute"
	"github.com/atlassian/smith/pkg/app"
	"github.com/atlassian/smith/pkg/resources"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	unstructured_conversion "k8s.io/apimachinery/pkg/conversion/unstructured"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	apiv1 "k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

const (
	useNamespace = metav1.NamespaceDefault
)

type testFunc func(*testing.T, context.Context, *smith.Bundle, *rest.Config, *kubernetes.Clientset, dynamic.ClientPool, *rest.RESTClient, *resources.Store, ...interface{})

func assertCondition(t *testing.T, bundle *smith.Bundle, conditionType smith.BundleConditionType, status smith.ConditionStatus) {
	_, condition := bundle.GetCondition(conditionType)
	if assert.NotNil(t, condition) {
		assert.Equal(t, status, condition.Status)
	}
}

func sleeperScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	scheme.AddUnversionedTypes(apiv1.SchemeGroupVersion, &metav1.Status{})
	tprattribute.AddToScheme(scheme)
	return scheme
}

func bundleInformer(bundleClient cache.Getter) cache.SharedIndexInformer {
	return cache.NewSharedIndexInformer(
		cache.NewListWatchFromClient(bundleClient, smith.BundleResourcePath, metav1.NamespaceAll, fields.Everything()),
		&smith.Bundle{},
		0,
		cache.Indexers{})
}

func cleanupBundle(t *testing.T, bundleClient *rest.RESTClient, clients dynamic.ClientPool, bundleCreated *bool, bundle *smith.Bundle) {
	if !*bundleCreated {
		return
	}
	t.Logf("Deleting bundle %s", bundle.Metadata.Name)
	assert.NoError(t, bundleClient.Delete().
		Namespace(bundle.Metadata.Namespace).
		Resource(smith.BundleResourcePath).
		Name(bundle.Metadata.Name).
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
		plural, _ := meta.KindToResource(resource.Spec.GroupVersionKind())
		assert.NoError(t, client.Resource(&metav1.APIResource{
			Name:       plural.Resource,
			Namespaced: true,
			Kind:       resource.Spec.GetKind(),
		}, bundle.Metadata.Namespace).Delete(resource.Spec.GetName(), nil))
	}
}

func isBundleReady(obj runtime.Object) bool {
	b := obj.(*smith.Bundle)
	_, cond := b.GetCondition(smith.BundleReady)
	return cond != nil && cond.Status == smith.ConditionTrue
}

func testSetup(t *testing.T) (*rest.Config, *kubernetes.Clientset, dynamic.ClientPool, *rest.RESTClient) {
	config, err := resources.ConfigFromEnv()
	require.NoError(t, err)

	clientset, err := kubernetes.NewForConfig(config)
	require.NoError(t, err)

	scheme := resources.BundleScheme()
	bundleClient, err := resources.GetBundleTprClient(config, scheme)
	require.NoError(t, err)

	clients := dynamic.NewClientPool(config, nil, dynamic.LegacyAPIPathResolverFunc)

	return config, clientset, clients, bundleClient
}

func setupApp(t *testing.T, bundle *smith.Bundle, serviceCatalog bool, test testFunc, args ...interface{}) {
	config, clientset, clients, bundleClient := testSetup(t)

	scheme, err := resources.FullScheme(serviceCatalog)
	require.NoError(t, err)

	store := resources.NewStore(scheme.DeepCopy)
	var wgStore sync.WaitGroup
	defer wgStore.Wait() // await store termination
	ctxStore, cancelStore := context.WithCancel(context.Background())
	defer cancelStore() // signal store to stop
	wgStore.Add(1)
	go store.Run(ctxStore, wgStore.Done)

	err = bundleClient.Delete().
		Namespace(bundle.Metadata.Namespace).
		Resource(smith.BundleResourcePath).
		Name(bundle.Metadata.Name).
		Do().
		Error()
	if err == nil {
		t.Log("Bundle deleted")
	} else if !kerrors.IsNotFound(err) {
		require.NoError(t, err)
	}
	var bundleCreated bool
	defer cleanupBundle(t, bundleClient, clients, &bundleCreated, bundle)

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

	time.Sleep(500 * time.Millisecond) // Wait until the app starts and creates the Bundle TPR

	t.Logf("Creating a new bundle %s/%s", bundle.Metadata.Namespace, bundle.Metadata.Name)
	require.NoError(t, bundleClient.Post().
		Namespace(bundle.Metadata.Namespace).
		Resource(smith.BundleResourcePath).
		Body(bundle).
		Do().
		Error())

	bundleCreated = true

	bundleInf := bundleInformer(bundleClient)
	store.AddInformer(smith.BundleGVK, bundleInf)
	go bundleInf.Run(ctx.Done())

	test(t, ctx, bundle, config, clientset, clients, bundleClient, store, args...)
}

func toUnstructured(t *testing.T, obj runtime.Object) unstructured.Unstructured {
	result := unstructured.Unstructured{
		Object: make(map[string]interface{}),
	}
	require.NoError(t, unstructured_conversion.NewConverter(true).ToUnstructured(obj, &result.Object))
	return result
}
