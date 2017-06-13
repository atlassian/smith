package integration_tests

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/atlassian/smith"
	"github.com/atlassian/smith/examples/tprattribute"
	"github.com/atlassian/smith/pkg/app"
	"github.com/atlassian/smith/pkg/client"
	"github.com/atlassian/smith/pkg/client/smart"
	"github.com/atlassian/smith/pkg/resources"
	"github.com/atlassian/smith/pkg/util/wait"

	scClientset "github.com/kubernetes-incubator/service-catalog/pkg/client/clientset_generated/clientset"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	createdBundle *smith.Bundle
	config        *rest.Config
	clientset     *kubernetes.Clientset
	sc            smith.SmartClient
	bundleClient  *rest.RESTClient
	store         *resources.Store
}

func assertCondition(t *testing.T, bundle *smith.Bundle, conditionType smith.BundleConditionType, status smith.ConditionStatus) *smith.BundleCondition {
	_, condition := bundle.GetCondition(conditionType)
	if assert.NotNil(t, condition) {
		assert.Equal(t, status, condition.Status)
	}
	return condition
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
	if cfg.createdBundle == nil {
		t.Logf("Not deleting bundle %s", cfg.bundle.Name)
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
		m, err := meta.Accessor(resource.Spec)
		if !assert.NoError(t, err) {
			continue
		}
		t.Logf("Deleting resource %q", m.GetName())
		client, err := cfg.sc.ForGVK(resource.Spec.GetObjectKind().GroupVersionKind(), cfg.namespace)
		if !assert.NoError(t, err) {
			continue
		}
		err = client.Delete(m.GetName(), nil)
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

func isBundleError(obj runtime.Object) bool {
	b := obj.(*smith.Bundle)
	_, cond := b.GetCondition(smith.BundleError)
	return cond != nil && cond.Status == smith.ConditionTrue
}

func isBundleReadyAndNewer(resourceVersions ...string) resources.AwaitCondition {
	return func(obj runtime.Object) bool {
		b := obj.(*smith.Bundle)
		for _, rv := range resourceVersions {
			if b.ResourceVersion == rv {
				// TODO Should be using Generation here once it is available
				// https://github.com/kubernetes/kubernetes/issues/7328
				// https://github.com/kubernetes/features/issues/95
				return false
			}
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

	scheme := client.BundleScheme()
	bundleClient, err := client.BundleClient(config, scheme)
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

	sc := smart.NewClient(config, scConfig, clientset, scClient)

	scheme, err := app.FullScheme(serviceCatalog)
	require.NoError(t, err)

	store := resources.NewStore(scheme.DeepCopy)
	var wgStore wait.Group
	defer wgStore.Wait() // await store termination
	ctxStore, cancelStore := context.WithCancel(context.Background())
	defer cancelStore() // signal store to stop
	wgStore.StartWithContext(ctxStore, store.Run)

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

	var wg wait.Group
	defer wg.Wait()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	wg.Start(func() {
		apl := app.App{
			RestConfig:           config,
			ServiceCatalogConfig: scConfig,
		}
		if e := apl.Run(ctx); e != context.Canceled && e != context.DeadlineExceeded {
			assert.NoError(t, e)
		}
	})

	if createBundle {
		time.Sleep(500 * time.Millisecond) // Wait until the app starts and creates the Bundle TPR
		res := &smith.Bundle{}
		createObject(t, bundle, res, useNamespace, smith.BundleResourcePath, bundleClient)
		cfg.createdBundle = res
	}

	bundleInf := bundleInformer(bundleClient, useNamespace)
	store.AddInformer(smith.BundleGVK, bundleInf)
	wg.StartWithChannel(ctx.Done(), bundleInf.Run)

	test(t, ctx, cfg, args...)
}

func createObject(t *testing.T, obj, res runtime.Object, namespace, resourcePath string, client *rest.RESTClient) {
	metaObj, err := meta.Accessor(obj)
	require.NoError(t, err)

	t.Logf("Creating a new object %s/%s of kind %s", namespace, metaObj.GetName(), obj.GetObjectKind().GroupVersionKind().Kind)
	require.NoError(t, client.Post().
		Namespace(namespace).
		Resource(resourcePath).
		Body(obj).
		Do().
		Into(res))
}

func assertBundle(t *testing.T, ctx context.Context, store *resources.Store, namespace string, bundle *smith.Bundle, resourceVersions ...string) *smith.Bundle {
	obj, err := store.AwaitObjectCondition(ctx, smith.BundleGVK, namespace, bundle.Name, isBundleReadyAndNewer(resourceVersions...))
	require.NoError(t, err)
	bundleRes := obj.(*smith.Bundle)

	assertCondition(t, bundleRes, smith.BundleReady, smith.ConditionTrue)
	assertCondition(t, bundleRes, smith.BundleInProgress, smith.ConditionFalse)
	assertCondition(t, bundleRes, smith.BundleError, smith.ConditionFalse)
	if assert.Len(t, bundleRes.Spec.Resources, len(bundle.Spec.Resources), "%#v", bundleRes) {
		for i, res := range bundle.Spec.Resources {
			spec, err := res.ToUnstructured(noCopy)
			if !assert.NoError(t, err) {
				continue
			}
			actual, err := bundleRes.Spec.Resources[i].ToUnstructured(noCopy)
			if !assert.NoError(t, err) {
				continue
			}
			assert.Equal(t, spec, actual, "%#v", bundleRes)
		}
	}

	return bundleRes
}

func assertBundleTimeout(t *testing.T, ctx context.Context, store *resources.Store, namespace string, bundle *smith.Bundle, resourceVersion ...string) *smith.Bundle {
	ctxTimeout, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return assertBundle(t, ctxTimeout, store, namespace, bundle, resourceVersion...)
}

// noCopy is a noop implementation of DeepCopy.
// Can be used when a real copy is not needed.
func noCopy(src interface{}) (interface{}, error) {
	return src, nil
}
