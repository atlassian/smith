package integration_tests

import (
	"context"
	"os"
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
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	apiv1 "k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

const (
	useNamespace              = metav1.NamespaceDefault
	serviceCatalogUrlEnvParam = "SERVICE_CATALOG_URL"
)

type testFunc func(*testing.T, context.Context, string, *smith.Bundle, *rest.Config, *kubernetes.Clientset, dynamic.ClientPool, *rest.RESTClient, *bool, *resources.Store, ...interface{})

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

func cleanupBundle(t *testing.T, namespace string, bundleClient *rest.RESTClient, clients, scDynamic dynamic.ClientPool, bundleCreated *bool, bundle *smith.Bundle) {
	if !*bundleCreated {
		return
	}
	t.Logf("Deleting bundle %s", bundle.Metadata.Name)
	assert.NoError(t, bundleClient.Delete().
		Namespace(namespace).
		Resource(smith.BundleResourcePath).
		Name(bundle.Metadata.Name).
		Do().
		Error())
	for _, resource := range bundle.Spec.Resources {
		t.Logf("Deleting resource %s", resource.Spec.GetName())
		client, err := resources.ClientForResource(&resource.Spec, clients, scDynamic, namespace)
		if !assert.NoError(t, err) {
			continue
		}
		assert.NoError(t, client.Delete(resource.Spec.GetName(), nil))
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

func setupApp(t *testing.T, bundle *smith.Bundle, serviceCatalog, createBundle bool, test testFunc, args ...interface{}) {
	config, clientset, clients, bundleClient := testSetup(t)
	var serviceCatalogConfig *rest.Config
	var scDynamic dynamic.ClientPool
	if serviceCatalog {
		scConfig := *config // shallow copy
		scConfig.Host = os.Getenv(serviceCatalogUrlEnvParam)
		require.NotEmpty(t, scConfig.Host, "required environment variable %s is not set", serviceCatalogUrlEnvParam)
		serviceCatalogConfig = &scConfig
		scDynamic = dynamic.NewClientPool(serviceCatalogConfig, nil, dynamic.LegacyAPIPathResolverFunc)
	}

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
		Namespace(useNamespace).
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
	defer cleanupBundle(t, useNamespace, bundleClient, clients, scDynamic, &bundleCreated, bundle)

	var wg sync.WaitGroup
	defer wg.Wait()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	wg.Add(1)
	go func() {
		defer wg.Done()
		apl := app.App{
			RestConfig:           config,
			ServiceCatalogConfig: serviceCatalogConfig,
		}
		if e := apl.Run(ctx); e != context.Canceled && e != context.DeadlineExceeded {
			assert.NoError(t, e)
		}
	}()

	if createBundle {
		time.Sleep(500 * time.Millisecond) // Wait until the app starts and creates the Bundle TPR

		createObject(t, bundle, useNamespace, smith.BundleResourcePath, bundleClient)
		bundleCreated = true
	}

	bundleInf := bundleInformer(bundleClient)
	store.AddInformer(smith.BundleGVK, bundleInf)
	go bundleInf.Run(ctx.Done())

	test(t, ctx, useNamespace, bundle, config, clientset, clients, bundleClient, &bundleCreated, store, args...)
}

func toUnstructured(t *testing.T, obj runtime.Object) unstructured.Unstructured {
	result := unstructured.Unstructured{
		Object: make(map[string]interface{}),
	}
	require.NoError(t, unstructured_conversion.NewConverter(true).ToUnstructured(obj, &result.Object))
	return result
}

func createObject(t *testing.T, obj runtime.Object, namespace, resourcePath string, client *rest.RESTClient) {
	metaObj, err := meta.Accessor(obj)
	require.NoError(t, err)

	t.Logf("Creating a new object %s/%s of kind %s", namespace, metaObj.GetName(), obj.GetObjectKind().GroupVersionKind().Kind)
	require.NoError(t, client.Post().
		Namespace(namespace).
		Resource(resourcePath).
		Body(obj).
		Do().
		Error())
}
