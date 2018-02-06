package it

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/atlassian/smith"
	"github.com/atlassian/smith/cmd/smith/app"
	"github.com/atlassian/smith/examples/sleeper"
	sleeper_v1 "github.com/atlassian/smith/examples/sleeper/pkg/apis/sleeper/v1"
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/smith/pkg/client"
	smithClientset "github.com/atlassian/smith/pkg/client/clientset_generated/clientset"
	"github.com/atlassian/smith/pkg/client/smart"
	"github.com/atlassian/smith/pkg/resources"
	"github.com/atlassian/smith/pkg/resources/apitypes"
	"github.com/atlassian/smith/pkg/util"
	smith_testing "github.com/atlassian/smith/pkg/util/testing"

	"github.com/ash2k/stager"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	core_v1 "k8s.io/api/core/v1"
	crdClientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apiext_v1b1inf "k8s.io/apiextensions-apiserver/pkg/client/informers/externalversions/apiextensions/v1beta1"
	apiext_v1b1list "k8s.io/apiextensions-apiserver/pkg/client/listers/apiextensions/v1beta1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

type TestFunc func(context.Context, *testing.T, *Config, ...interface{})

type Config struct {
	T             *testing.T
	Logger        *zap.Logger
	Namespace     string
	Bundle        *smith_v1.Bundle
	CreatedBundle *smith_v1.Bundle
	Config        *rest.Config
	Clientset     kubernetes.Interface
	Sc            smith.SmartClient
	BundleClient  smithClientset.Interface
	Scheme        *runtime.Scheme
}

func (cfg *Config) CreateObject(ctxTest context.Context, obj, res runtime.Object, resourcePath string, client rest.Interface) {
	metaObj := obj.(meta_v1.Object)

	cfg.T.Logf("Creating a new object %s/%s of kind %s", cfg.Namespace, metaObj.GetName(), obj.GetObjectKind().GroupVersionKind().Kind)
	require.NoError(cfg.T, client.Post().
		Context(ctxTest).
		Namespace(cfg.Namespace).
		Resource(resourcePath).
		Body(obj).
		Do().
		Into(res))
}

func (cfg *Config) AwaitBundleCondition(conditions ...watch.ConditionFunc) *smith_v1.Bundle {
	lw := cache.NewListWatchFromClient(cfg.BundleClient.SmithV1().RESTClient(), smith_v1.BundleResourcePlural, cfg.Namespace, fields.Everything())
	event, err := cache.ListWatchUntil(10*time.Second, lw, conditions...)
	require.NoError(cfg.T, err)
	return event.Object.(*smith_v1.Bundle)
}

func SleeperScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	scheme.AddUnversionedTypes(core_v1.SchemeGroupVersion, &meta_v1.Status{})
	if err := sleeper_v1.AddToScheme(scheme); err != nil {
		panic(err)
	}
	return scheme
}

func IsBundleStatusCond(namespace, name string, cType smith_v1.BundleConditionType, status smith_v1.ConditionStatus) watch.ConditionFunc {
	return func(event watch.Event) (bool, error) {
		metaObj := event.Object.(meta_v1.Object)
		if metaObj.GetNamespace() != namespace || metaObj.GetName() != name {
			return false, nil
		}
		switch event.Type {
		case watch.Added, watch.Modified:
			b := event.Object.(*smith_v1.Bundle)
			_, cond := b.GetCondition(cType)
			return cond != nil && cond.Status == status, nil
		default:
			return false, errors.Errorf("unexpected event type %q: %v", event.Type, event.Object)
		}
	}
}

func IsBundleNewerCond(namespace, name string, resourceVersions ...string) watch.ConditionFunc {
	return func(event watch.Event) (bool, error) {
		metaObj := event.Object.(meta_v1.Object)
		if metaObj.GetNamespace() != namespace || metaObj.GetName() != name {
			return false, nil
		}
		b := event.Object.(*smith_v1.Bundle)
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

func TestSetup(t *testing.T) (*rest.Config, *kubernetes.Clientset, *smithClientset.Clientset) {
	configFileFrom := os.Getenv("KUBERNETES_CONFIG_FROM")
	configFileName := os.Getenv("KUBERNETES_CONFIG_FILENAME")
	configContext := os.Getenv("KUBERNETES_CONFIG_CONTEXT")

	config, err := client.LoadConfig(configFileFrom, configFileName, configContext)
	require.NoError(t, err)

	clientset, err := kubernetes.NewForConfig(config)
	require.NoError(t, err)

	bundleClient, err := smithClientset.NewForConfig(config)
	require.NoError(t, err)

	return config, clientset, bundleClient
}

func SetupApp(t *testing.T, bundle *smith_v1.Bundle, serviceCatalog, createBundle bool, test TestFunc, args ...interface{}) {
	scheme, err := apitypes.FullScheme(serviceCatalog)
	require.NoError(t, err)
	err = sleeper_v1.AddToScheme(scheme)
	require.NoError(t, err)

	// Convert all typed objects into unstructured ones
	for i, res := range bundle.Spec.Resources {
		if res.Spec.Object != nil {
			resUnstr, err := util.RuntimeToUnstructured(scheme, res.Spec.Object)
			require.NoError(t, err)
			bundle.Spec.Resources[i].Spec.Object = resUnstr
		}
	}
	config, clientset, bundleClient := TestSetup(t)

	sc := smart.NewClient(config, clientset)

	logger, err := zap.NewDevelopment()
	require.NoError(t, err)
	defer logger.Sync()

	cfg := &Config{
		T:            t,
		Logger:       logger,
		Namespace:    fmt.Sprintf("smith-it-%d", rand.Uint32()),
		Bundle:       bundle,
		Config:       config,
		Clientset:    clientset,
		Sc:           sc,
		BundleClient: bundleClient,
		Scheme:       scheme,
	}

	t.Logf("Creating namespace %q", cfg.Namespace)
	_, err = clientset.CoreV1().Namespaces().Create(&core_v1.Namespace{
		ObjectMeta: meta_v1.ObjectMeta{
			Name: cfg.Namespace,
		},
	})
	require.NoError(t, err)

	defer func() {
		t.Logf("Deleting namespace %q", cfg.Namespace)
		assert.NoError(t, clientset.CoreV1().Namespaces().Delete(cfg.Namespace, nil))
	}()

	stgr := stager.New()
	defer stgr.Shutdown()

	ctxTest, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()

	crdClient, err := crdClientset.NewForConfig(config)
	require.NoError(t, err)

	crdInf := apiext_v1b1inf.NewCustomResourceDefinitionInformer(crdClient, 0, cache.Indexers{})
	stage := stgr.NextStage()
	stage.StartWithChannel(crdInf.Run)

	// We must wait for crdInf to populate its cache to avoid reading from an empty cache
	// in resources.EnsureCrdExistsAndIsEstablished().
	if !cache.WaitForCacheSync(ctxTest.Done(), crdInf.HasSynced) {
		t.Fatal("wait for CRD Informer was cancelled")
	}

	crdLister := apiext_v1b1list.NewCustomResourceDefinitionLister(crdInf.GetIndexer())
	require.NoError(t, resources.EnsureCrdExistsAndIsEstablished(ctxTest, logger, scheme, crdClient, crdLister, sleeper.SleeperCrd()))
	require.NoError(t, resources.EnsureCrdExistsAndIsEstablished(ctxTest, logger, scheme, crdClient, crdLister, resources.BundleCrd()))

	stage.StartWithContext(func(ctx context.Context) {
		apl := app.App{
			Logger:                logger,
			RestConfig:            config,
			ServiceCatalogSupport: serviceCatalog,
			Namespace:             cfg.Namespace,
			Workers:               2,
		}
		if e := apl.Run(ctx); e != context.Canceled && e != context.DeadlineExceeded {
			assert.NoError(t, e)
		}
	})

	if createBundle {
		res := &smith_v1.Bundle{}
		cfg.CreateObject(ctxTest, bundle, res, smith_v1.BundleResourcePlural, bundleClient.SmithV1().RESTClient())
		cfg.CreatedBundle = res
	}

	test(ctxTest, t, cfg, args...)
}

func (cfg *Config) AssertBundle(ctx context.Context, bundle *smith_v1.Bundle, resourceVersions ...string) *smith_v1.Bundle {
	bundleRes := cfg.AwaitBundleCondition(IsBundleNewerCond(cfg.Namespace, bundle.Name, resourceVersions...), IsBundleStatusCond(cfg.Namespace, cfg.Bundle.Name, smith_v1.BundleReady, smith_v1.ConditionTrue))

	smith_testing.AssertCondition(cfg.T, bundleRes, smith_v1.BundleReady, smith_v1.ConditionTrue)
	smith_testing.AssertCondition(cfg.T, bundleRes, smith_v1.BundleInProgress, smith_v1.ConditionFalse)
	smith_testing.AssertCondition(cfg.T, bundleRes, smith_v1.BundleError, smith_v1.ConditionFalse)
	if assert.Len(cfg.T, bundleRes.Spec.Resources, len(bundle.Spec.Resources), "%#v", bundleRes) {
		for i, res := range bundle.Spec.Resources {
			spec, err := util.RuntimeToUnstructured(cfg.Scheme, res.Spec.Object)
			if !assert.NoError(cfg.T, err) {
				continue
			}
			actual, err := util.RuntimeToUnstructured(cfg.Scheme, bundleRes.Spec.Resources[i].Spec.Object)
			if !assert.NoError(cfg.T, err) {
				continue
			}
			assert.Equal(cfg.T, spec, actual, "%#v", bundleRes)
			smith_testing.AssertResourceCondition(cfg.T, bundleRes, res.Name, smith_v1.ResourceBlocked, smith_v1.ConditionFalse)
			smith_testing.AssertResourceCondition(cfg.T, bundleRes, res.Name, smith_v1.ResourceInProgress, smith_v1.ConditionFalse)
			smith_testing.AssertResourceCondition(cfg.T, bundleRes, res.Name, smith_v1.ResourceReady, smith_v1.ConditionTrue)
			smith_testing.AssertResourceCondition(cfg.T, bundleRes, res.Name, smith_v1.ResourceError, smith_v1.ConditionFalse)
		}
	}

	return bundleRes
}

func (cfg *Config) AssertBundleTimeout(ctx context.Context, bundle *smith_v1.Bundle, resourceVersion ...string) *smith_v1.Bundle {
	ctxTimeout, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return cfg.AssertBundle(ctxTimeout, bundle, resourceVersion...)
}
