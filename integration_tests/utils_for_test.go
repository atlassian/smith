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
	"github.com/atlassian/smith/pkg/util"

	scClientset "github.com/kubernetes-incubator/service-catalog/pkg/client/clientset_generated/clientset"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	unstructured_conversion "k8s.io/apimachinery/pkg/conversion/unstructured"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	api_v1 "k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

const (
	useNamespace              = meta_v1.NamespaceDefault
	serviceCatalogUrlEnvParam = "SERVICE_CATALOG_URL"
)

type testFunc func(*testing.T, context.Context, *itConfig, ...interface{})

type itConfig struct {
	namespace     string
	bundle        *smith.Bundle
	config        *rest.Config
	clientset     *kubernetes.Clientset
	sc            smith.SmartClient
	bundleClient  *rest.RESTClient
	store         *resources.Store
	bundleCreated bool
}

func assertCondition(t *testing.T, bundle *smith.Bundle, conditionType smith.BundleConditionType, status smith.ConditionStatus) {
	_, condition := bundle.GetCondition(conditionType)
	if assert.NotNil(t, condition) {
		assert.Equal(t, status, condition.Status)
	}
}

func sleeperScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	scheme.AddUnversionedTypes(api_v1.SchemeGroupVersion, &meta_v1.Status{})
	tprattribute.AddToScheme(scheme)
	return scheme
}

func bundleInformer(bundleClient cache.Getter, namespace string) cache.SharedIndexInformer {
	return cache.NewSharedIndexInformer(
		cache.NewListWatchFromClient(bundleClient, smith.BundleResourcePath, namespace, fields.Everything()),
		&smith.Bundle{},
		0,
		cache.Indexers{})
}

func cleanupBundle(t *testing.T, cfg *itConfig) {
	if !cfg.bundleCreated {
		return
	}
	t.Logf("Deleting bundle %s", cfg.bundle.Name)
	err := cfg.bundleClient.Delete().
		Namespace(cfg.namespace).
		Resource(smith.BundleResourcePath).
		Name(cfg.bundle.Name).
		Do().
		Error()
	if !kerrors.IsNotFound(err) {
		assert.NoError(t, err)
	}
	for _, resource := range cfg.bundle.Spec.Resources {
		t.Logf("Deleting resource %q", resource.Spec.GetName())
		client, err := cfg.sc.ClientForGVK(resource.Spec.GroupVersionKind(), cfg.namespace)
		if !assert.NoError(t, err) {
			continue
		}
		err = client.Delete(resource.Spec.GetName(), nil)
		if !kerrors.IsNotFound(err) {
			assert.NoError(t, err)
		}
	}
}

func isBundleReady(obj runtime.Object) bool {
	b := obj.(*smith.Bundle)
	_, cond := b.GetCondition(smith.BundleReady)
	return cond != nil && cond.Status == smith.ConditionTrue
}

func isBundleReadyAndNewer(resourceVersion string) resources.AwaitCondition {
	return func(obj runtime.Object) bool {
		b := obj.(*smith.Bundle)
		if b.ResourceVersion == resourceVersion {
			// TODO Should be using Generation here once it is available
			// https://github.com/kubernetes/kubernetes/issues/7328
			// https://github.com/kubernetes/features/issues/95
			return false
		}
		_, cond := b.GetCondition(smith.BundleReady)
		return cond != nil && cond.Status == smith.ConditionTrue
	}
}

func testSetup(t *testing.T) (*rest.Config, *kubernetes.Clientset, *rest.RESTClient) {
	config, err := resources.ConfigFromEnv()
	require.NoError(t, err)

	clientset, err := kubernetes.NewForConfig(config)
	require.NoError(t, err)

	scheme := resources.BundleScheme()
	bundleClient, err := resources.BundleClient(config, scheme)
	require.NoError(t, err)

	return config, clientset, bundleClient
}

func setupApp(t *testing.T, bundle *smith.Bundle, serviceCatalog, createBundle bool, test testFunc, args ...interface{}) {
	config, clientset, bundleClient := testSetup(t)
	var scConfig *rest.Config
	var scClient scClientset.Interface
	if serviceCatalog {
		scConfigTmp := *config // shallow copy
		scConfigTmp.Host = os.Getenv(serviceCatalogUrlEnvParam)
		require.NotEmpty(t, scConfigTmp.Host, "required environment variable %s is not set", serviceCatalogUrlEnvParam)
		scConfig = &scConfigTmp
		var err error
		scClient, err = scClientset.NewForConfig(scConfig)
		require.NoError(t, err)
	}

	sc := resources.NewSmartClient(config, scConfig, clientset, scClient)

	scheme, err := resources.FullScheme(serviceCatalog)
	require.NoError(t, err)

	store := resources.NewStore(scheme.DeepCopy)
	var wgStore sync.WaitGroup
	defer wgStore.Wait() // await store termination
	ctxStore, cancelStore := context.WithCancel(context.Background())
	defer cancelStore() // signal store to stop
	util.StartAsync(ctxStore, &wgStore, store.Run)

	err = bundleClient.Delete().
		Namespace(useNamespace).
		Resource(smith.BundleResourcePath).
		Name(bundle.Name).
		Do().
		Error()
	if err == nil {
		t.Log("Bundle deleted")
	} else if !kerrors.IsNotFound(err) {
		require.NoError(t, err)
	}
	cfg := &itConfig{
		namespace:    useNamespace,
		bundle:       bundle,
		config:       config,
		clientset:    clientset,
		sc:           sc,
		bundleClient: bundleClient,
		store:        store,
	}
	defer cleanupBundle(t, cfg)

	var wg sync.WaitGroup
	defer wg.Wait()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	wg.Add(1)
	go func() {
		defer wg.Done()
		apl := app.App{
			RestConfig:           config,
			ServiceCatalogConfig: scConfig,
		}
		if e := apl.Run(ctx); e != context.Canceled && e != context.DeadlineExceeded {
			assert.NoError(t, e)
		}
	}()

	if createBundle {
		time.Sleep(500 * time.Millisecond) // Wait until the app starts and creates the Bundle TPR

		createObject(t, bundle, useNamespace, smith.BundleResourcePath, bundleClient)
		cfg.bundleCreated = true
	}

	bundleInf := bundleInformer(bundleClient, useNamespace)
	store.AddInformer(smith.BundleGVK, bundleInf)
	go bundleInf.Run(ctx.Done())

	test(t, ctx, cfg, args...)
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

func assertBundle(t *testing.T, ctx context.Context, store *resources.Store, namespace string, bundle *smith.Bundle, resourceVersion string) *smith.Bundle {
	obj, err := store.AwaitObjectCondition(ctx, smith.BundleGVK, namespace, bundle.Name, isBundleReadyAndNewer(resourceVersion))
	require.NoError(t, err)
	bundleRes := obj.(*smith.Bundle)

	assertCondition(t, bundleRes, smith.BundleReady, smith.ConditionTrue)
	assertCondition(t, bundleRes, smith.BundleInProgress, smith.ConditionFalse)
	assertCondition(t, bundleRes, smith.BundleError, smith.ConditionFalse)
	assert.Equal(t, bundle.Spec, bundleRes.Spec, "%#v", bundleRes)

	return bundleRes
}

func assertBundleTimeout(t *testing.T, ctx context.Context, store *resources.Store, namespace string, bundle *smith.Bundle, resourceVersion string) *smith.Bundle {
	ctxTimeout, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return assertBundle(t, ctxTimeout, store, namespace, bundle, resourceVersion)
}
