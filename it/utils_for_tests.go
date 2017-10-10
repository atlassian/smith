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

	"github.com/ash2k/stager"
	scClientset "github.com/kubernetes-incubator/service-catalog/pkg/client/clientset_generated/clientset"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	api_v1 "k8s.io/api/core/v1"
	crdClientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	crdInformers "k8s.io/apiextensions-apiserver/pkg/client/informers/externalversions"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

const (
	serviceCatalogUrlEnvParam = "SERVICE_CATALOG_URL"
)

type TestFunc func(*testing.T, context.Context, *ItConfig, ...interface{})

type ItConfig struct {
	T             *testing.T
	Namespace     string
	Bundle        *smith_v1.Bundle
	CreatedBundle *smith_v1.Bundle
	Config        *rest.Config
	Clientset     kubernetes.Interface
	Sc            smith.SmartClient
	BundleClient  smithClientset.Interface
}

func (cfg *ItConfig) CreateObject(ctxTest context.Context, obj, res runtime.Object, resourcePath string, client rest.Interface) {
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

func (cfg *ItConfig) AwaitBundleCondition(conditions ...watch.ConditionFunc) *smith_v1.Bundle {
	lw := cache.NewListWatchFromClient(cfg.BundleClient.SmithV1().RESTClient(), smith_v1.BundleResourcePlural, cfg.Namespace, fields.Everything())
	event, err := cache.ListWatchUntil(10*time.Second, lw, conditions...)
	require.NoError(cfg.T, err)
	return event.Object.(*smith_v1.Bundle)
}

func AssertCondition(t *testing.T, bundle *smith_v1.Bundle, conditionType smith_v1.BundleConditionType, status smith_v1.ConditionStatus) *smith_v1.BundleCondition {
	_, condition := bundle.GetCondition(conditionType)
	if assert.NotNil(t, condition) {
		assert.Equal(t, status, condition.Status)
	}
	return condition
}

func SleeperScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	scheme.AddUnversionedTypes(api_v1.SchemeGroupVersion, &meta_v1.Status{})
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
			return false, fmt.Errorf("unexpected event type %q: %v", event.Type, event.Object)
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
	config, err := client.ConfigFromEnv()
	require.NoError(t, err)

	clientset, err := kubernetes.NewForConfig(config)
	require.NoError(t, err)

	bundleClient, err := smithClientset.NewForConfig(config)
	require.NoError(t, err)

	return config, clientset, bundleClient
}

func SetupApp(t *testing.T, bundle *smith_v1.Bundle, serviceCatalog, createBundle bool, test TestFunc, args ...interface{}) {
	config, clientset, bundleClient := TestSetup(t)
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

	scheme, err := apitypes.FullScheme(serviceCatalog)
	require.NoError(t, err)

	cfg := &ItConfig{
		T:            t,
		Namespace:    fmt.Sprintf("smith-it-%d", rand.Uint32()),
		Bundle:       bundle,
		Config:       config,
		Clientset:    clientset,
		Sc:           sc,
		BundleClient: bundleClient,
	}

	t.Logf("Creating namespace %q", cfg.Namespace)
	_, err = clientset.CoreV1().Namespaces().Create(&api_v1.Namespace{
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

	informerFactory := crdInformers.NewSharedInformerFactory(crdClient, 0)
	crdInf := informerFactory.Apiextensions().V1beta1().CustomResourceDefinitions().Informer()
	stage := stgr.NextStage()
	stage.StartWithChannel(crdInf.Run)

	// We must wait for crdInf to populate its cache to avoid reading from an empty cache
	// in resources.EnsureCrdExistsAndIsEstablished().
	if !cache.WaitForCacheSync(ctxTest.Done(), crdInf.HasSynced) {
		t.Fatal("wait for CRD Informer was cancelled")
	}

	crdLister := informerFactory.Apiextensions().V1beta1().CustomResourceDefinitions().Lister()
	require.NoError(t, resources.EnsureCrdExistsAndIsEstablished(ctxTest, scheme, crdClient, crdLister, sleeper.SleeperCrd()))
	require.NoError(t, resources.EnsureCrdExistsAndIsEstablished(ctxTest, scheme, crdClient, crdLister, resources.BundleCrd()))

	stage.StartWithContext(func(ctx context.Context) {
		apl := app.App{
			RestConfig:           config,
			ServiceCatalogConfig: scConfig,
			Namespace:            cfg.Namespace,
			Workers:              2,
		}
		if e := apl.Run(ctx); e != context.Canceled && e != context.DeadlineExceeded {
			assert.NoError(t, e)
		}
	})

	if createBundle {
		time.Sleep(500 * time.Millisecond) // Wait until the app starts and creates the Bundle CRD
		res := &smith_v1.Bundle{}
		cfg.CreateObject(ctxTest, bundle, res, smith_v1.BundleResourcePlural, bundleClient.SmithV1().RESTClient())
		cfg.CreatedBundle = res
	}

	test(t, ctxTest, cfg, args...)
}

func (cfg *ItConfig) AssertBundle(ctx context.Context, bundle *smith_v1.Bundle, resourceVersions ...string) *smith_v1.Bundle {
	bundleRes := cfg.AwaitBundleCondition(IsBundleNewerCond(cfg.Namespace, bundle.Name, resourceVersions...), IsBundleStatusCond(cfg.Namespace, cfg.Bundle.Name, smith_v1.BundleReady, smith_v1.ConditionTrue))

	AssertCondition(cfg.T, bundleRes, smith_v1.BundleReady, smith_v1.ConditionTrue)
	AssertCondition(cfg.T, bundleRes, smith_v1.BundleInProgress, smith_v1.ConditionFalse)
	AssertCondition(cfg.T, bundleRes, smith_v1.BundleError, smith_v1.ConditionFalse)
	if assert.Len(cfg.T, bundleRes.Spec.Resources, len(bundle.Spec.Resources), "%#v", bundleRes) {
		for i, res := range bundle.Spec.Resources {
			spec, err := res.ToUnstructured(noCopy)
			if !assert.NoError(cfg.T, err) {
				continue
			}
			actual, err := bundleRes.Spec.Resources[i].ToUnstructured(noCopy)
			if !assert.NoError(cfg.T, err) {
				continue
			}
			assert.Equal(cfg.T, spec, actual, "%#v", bundleRes)
		}
	}

	return bundleRes
}

func (cfg *ItConfig) AssertBundleTimeout(ctx context.Context, bundle *smith_v1.Bundle, resourceVersion ...string) *smith_v1.Bundle {
	ctxTimeout, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return cfg.AssertBundle(ctxTimeout, bundle, resourceVersion...)
}

// noCopy is a noop implementation of DeepCopy.
// Can be used when a real copy is not needed.
func noCopy(src interface{}) (interface{}, error) {
	return src, nil
}
