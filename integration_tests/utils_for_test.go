package integration_tests

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/atlassian/smith"
	"github.com/atlassian/smith/cmd/smith/app"
	"github.com/atlassian/smith/examples/sleeper"
	"github.com/atlassian/smith/pkg/client"
	"github.com/atlassian/smith/pkg/client/smart"
	"github.com/atlassian/smith/pkg/resources"
	"github.com/atlassian/smith/pkg/store"

	"github.com/ash2k/stager"
	scClientset "github.com/kubernetes-incubator/service-catalog/pkg/client/clientset_generated/clientset"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apiext_v1b1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	crdClientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	crdInformers "k8s.io/apiextensions-apiserver/pkg/client/informers/externalversions"
	api_errors "k8s.io/apimachinery/pkg/api/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	unstructured_conversion "k8s.io/apimachinery/pkg/conversion/unstructured"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
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
	t             *testing.T
	namespace     string
	bundle        *smith.Bundle
	createdBundle *smith.Bundle
	config        *rest.Config
	clientset     kubernetes.Interface
	sc            smith.SmartClient
	bundleClient  rest.Interface
	store         *store.Multi
	toCleanup     []runtime.Object
}

func (cfg *itConfig) cleanupLater(obj ...runtime.Object) {
	cfg.toCleanup = append(cfg.toCleanup, obj...)
}

func (cfg *itConfig) cleanup() {
	for _, obj := range cfg.toCleanup {
		cfg.deleteObject(obj)
		bundle, ok := obj.(*smith.Bundle)
		if !ok {
			u, ok := obj.(*unstructured.Unstructured)
			if !ok || u.GetKind() != smith.BundleResourceKind || u.GetAPIVersion() != smith.BundleResourceGroupVersion {
				continue
			}
			bundle = new(smith.Bundle)
			if !assert.NoError(cfg.t, unstructured_conversion.DefaultConverter.FromUnstructured(u.Object, bundle)) {
				continue
			}
		}
		cfg.cleanupBundle(bundle)
	}
}
func (cfg *itConfig) cleanupBundle(bundle *smith.Bundle) {
	for _, resource := range bundle.Spec.Resources {
		cfg.deleteObject(resource.Spec)
	}
}

func (cfg *itConfig) deleteObject(obj runtime.Object) {
	m := obj.(meta_v1.Object)
	gvk := obj.GetObjectKind().GroupVersionKind()
	if gvk.Empty() {
		switch obj.(type) {
		case *api_v1.ConfigMap:
			gvk = api_v1.SchemeGroupVersion.WithKind("ConfigMap")
		case *api_v1.Secret:
			gvk = api_v1.SchemeGroupVersion.WithKind("Secret")
		case *smith.Bundle:
			gvk = smith.BundleGVK
		case *sleeper.Sleeper:
			gvk = sleeper.SleeperGVK
		default:
			assert.Fail(cfg.t, "Unhandled object kind", "%T", obj)
			return
		}
	}
	cfg.t.Logf("Deleting object %q", m.GetName())
	objClient, err := cfg.sc.ForGVK(gvk, cfg.namespace)
	if !assert.NoError(cfg.t, err) {
		return
	}
	policy := meta_v1.DeletePropagationForeground
	err = objClient.Delete(m.GetName(), &meta_v1.DeleteOptions{
		PropagationPolicy: &policy,
	})
	if !api_errors.IsNotFound(err) {
		assert.NoError(cfg.t, err)
	}
}

func (cfg *itConfig) createObject(ctxTest context.Context, obj, res runtime.Object, resourcePath string, client rest.Interface) {
	metaObj := obj.(meta_v1.Object)

	cfg.t.Logf("Creating a new object %s/%s of kind %s", cfg.namespace, metaObj.GetName(), obj.GetObjectKind().GroupVersionKind().Kind)
	require.NoError(cfg.t, client.Post().
		Context(ctxTest).
		Namespace(cfg.namespace).
		Resource(resourcePath).
		Body(obj).
		Do().
		Into(res))
	cfg.cleanupLater(res)
}

func (cfg *itConfig) awaitBundleCondition(conditions ...watch.ConditionFunc) *smith.Bundle {
	lw := cache.NewListWatchFromClient(cfg.bundleClient, smith.BundleResourcePlural, cfg.namespace, fields.Everything())
	event, err := cache.ListWatchUntil(10*time.Second, lw, conditions...)
	require.NoError(cfg.t, err)
	return event.Object.(*smith.Bundle)
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
	sleeper.AddToScheme(scheme)
	return scheme
}

func bundleInformer(bundleClient cache.Getter, namespace string) cache.SharedIndexInformer {
	return cache.NewSharedIndexInformer(
		cache.NewListWatchFromClient(bundleClient, smith.BundleResourcePlural, namespace, fields.Everything()),
		&smith.Bundle{},
		0,
		cache.Indexers{})
}

func isBundleStatusCond(cType smith.BundleConditionType, status smith.ConditionStatus) watch.ConditionFunc {
	return func(event watch.Event) (bool, error) {
		switch event.Type {
		case watch.Added, watch.Modified:
			b := event.Object.(*smith.Bundle)
			_, cond := b.GetCondition(cType)
			return cond != nil && cond.Status == status, nil
		default:
			return false, fmt.Errorf("unexpected event type %q: %v", event.Type, event.Object)
		}
	}
}

func isBundleNewerCond(resourceVersions ...string) watch.ConditionFunc {
	return func(event watch.Event) (bool, error) {
		b := event.Object.(*smith.Bundle)
		for _, rv := range resourceVersions {
			if b.ResourceVersion == rv {
				// TODO Should be using Generation here once it is available
				// https://github.com/kubernetes/kubernetes/issues/7328
				// https://github.com/kubernetes/features/issues/95
				return false, nil
			}
		}
		return true, nil
	}
}

func testSetup(t *testing.T) (*rest.Config, *kubernetes.Clientset, *rest.RESTClient) {
	config, err := client.ConfigFromEnv()
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

	multiStore := store.NewMulti(scheme.DeepCopy)
	cfg := &itConfig{
		t:            t,
		namespace:    useNamespace,
		bundle:       bundle,
		config:       config,
		clientset:    clientset,
		sc:           sc,
		bundleClient: bundleClient,
		store:        multiStore,
	}
	defer cfg.cleanup()

	stgr := stager.New()
	defer stgr.Shutdown()

	ctxTest, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()

	err = bundleClient.Delete().
		Context(ctxTest).
		Namespace(useNamespace).
		Resource(smith.BundleResourcePlural).
		Name(bundle.Name).
		Do().
		Error()
	if err == nil {
		t.Log("Bundle deleted")
	} else if !api_errors.IsNotFound(err) {
		require.NoError(t, err)
	}

	crdClient, err := crdClientset.NewForConfig(config)
	require.NoError(t, err)

	informerFactory := crdInformers.NewSharedInformerFactory(crdClient, 0)
	crdInf := informerFactory.Apiextensions().V1beta1().CustomResourceDefinitions().Informer()
	multiStore.AddInformer(apiext_v1b1.SchemeGroupVersion.WithKind("CustomResourceDefinition"), crdInf)
	stage := stgr.NextStage()
	stage.StartWithChannel(crdInf.Run)

	// We must wait for crdInf to populate its cache to avoid reading from an empty cache
	// in resources.EnsureCrdExists().
	if !cache.WaitForCacheSync(ctxTest.Done(), crdInf.HasSynced) {
		t.Fatal("wait for CRD Informer was cancelled")
	}

	crdLister := informerFactory.Apiextensions().V1beta1().CustomResourceDefinitions().Lister()
	require.NoError(t, resources.EnsureCrdExists(ctxTest, scheme, crdClient, crdLister, sleeper.SleeperCrd()))
	require.NoError(t, resources.EnsureCrdExists(ctxTest, scheme, crdClient, crdLister, resources.BundleCrd()))

	stage.StartWithContext(func(ctx context.Context) {
		apl := app.App{
			RestConfig:           config,
			ServiceCatalogConfig: scConfig,
			Workers:              2,
		}
		if e := apl.Run(ctx); e != context.Canceled && e != context.DeadlineExceeded {
			assert.NoError(t, e)
		}
	})

	if createBundle {
		time.Sleep(500 * time.Millisecond) // Wait until the app starts and creates the Bundle CRD
		res := &smith.Bundle{}
		cfg.createObject(ctxTest, bundle, res, smith.BundleResourcePlural, bundleClient)
		cfg.createdBundle = res
	}

	bundleInf := bundleInformer(bundleClient, useNamespace)
	multiStore.AddInformer(smith.BundleGVK, bundleInf)
	stage.StartWithChannel(bundleInf.Run)

	test(t, ctxTest, cfg, args...)
}

func (cfg *itConfig) assertBundle(ctx context.Context, bundle *smith.Bundle, resourceVersions ...string) *smith.Bundle {
	bundleRes := cfg.awaitBundleCondition(isBundleNewerCond(resourceVersions...), isBundleStatusCond(smith.BundleReady, smith.ConditionTrue))

	assertCondition(cfg.t, bundleRes, smith.BundleReady, smith.ConditionTrue)
	assertCondition(cfg.t, bundleRes, smith.BundleInProgress, smith.ConditionFalse)
	assertCondition(cfg.t, bundleRes, smith.BundleError, smith.ConditionFalse)
	if assert.Len(cfg.t, bundleRes.Spec.Resources, len(bundle.Spec.Resources), "%#v", bundleRes) {
		for i, res := range bundle.Spec.Resources {
			spec, err := res.ToUnstructured(noCopy)
			if !assert.NoError(cfg.t, err) {
				continue
			}
			actual, err := bundleRes.Spec.Resources[i].ToUnstructured(noCopy)
			if !assert.NoError(cfg.t, err) {
				continue
			}
			assert.Equal(cfg.t, spec, actual, "%#v", bundleRes)
		}
	}

	return bundleRes
}

func (cfg *itConfig) assertBundleTimeout(ctx context.Context, bundle *smith.Bundle, resourceVersion ...string) *smith.Bundle {
	ctxTimeout, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return cfg.assertBundle(ctxTimeout, bundle, resourceVersion...)
}

// noCopy is a noop implementation of DeepCopy.
// Can be used when a real copy is not needed.
func noCopy(src interface{}) (interface{}, error) {
	return src, nil
}
