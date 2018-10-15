package it

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/ash2k/stager"
	"github.com/atlassian/ctrl"
	cond_v1 "github.com/atlassian/ctrl/apis/condition/v1"
	ctrlApp "github.com/atlassian/ctrl/app"
	"github.com/atlassian/smith/cmd/smith/app"
	"github.com/atlassian/smith/examples/sleeper"
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/smith/pkg/client"
	smithClientset "github.com/atlassian/smith/pkg/client/clientset_generated/clientset"
	"github.com/atlassian/smith/pkg/client/smart"
	"github.com/atlassian/smith/pkg/resources"
	"github.com/atlassian/smith/pkg/util"
	smith_testing "github.com/atlassian/smith/pkg/util/testing"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
	core_v1 "k8s.io/api/core/v1"
	apiExtClientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apiext_v1b1inf "k8s.io/apiextensions-apiserver/pkg/client/informers/externalversions/apiextensions/v1beta1"
	apiext_v1b1list "k8s.io/apiextensions-apiserver/pkg/client/listers/apiextensions/v1beta1"
	api_errors "k8s.io/apimachinery/pkg/api/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
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
	MainClient    kubernetes.Interface
	Sc            *smart.DynamicClient
	SmithClient   smithClientset.Interface
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
	// Before checking any conditions, ensure that .status is up to date with .spec
	generation := IsBundleObservedGenerationCond(cfg.Namespace, cfg.Bundle.Name)
	for i, cond := range conditions {
		conditions[i] = AndCond(generation, cond)
	}
	lw := cache.NewListWatchFromClient(cfg.SmithClient.SmithV1().RESTClient(), smith_v1.BundleResourcePlural, cfg.Namespace, fields.Everything())
	event, err := cache.ListWatchUntil(20*time.Second, lw, conditions...)
	require.NoError(cfg.T, err)
	return event.Object.(*smith_v1.Bundle)
}

func AndCond(conds ...watch.ConditionFunc) watch.ConditionFunc {
	return func(event watch.Event) (bool, error) {
		for _, c := range conds {
			res, err := c(event)
			if err != nil || !res {
				return res, err
			}
		}
		return true, nil
	}
}

func IsBundleStatusCond(namespace, name string, cType cond_v1.ConditionType, status cond_v1.ConditionStatus) watch.ConditionFunc {
	return func(event watch.Event) (bool, error) {
		metaObj := event.Object.(meta_v1.Object)
		if metaObj.GetNamespace() != namespace || metaObj.GetName() != name {
			return false, nil
		}
		switch event.Type {
		case watch.Added, watch.Modified:
			b := event.Object.(*smith_v1.Bundle)
			_, cond := cond_v1.FindCondition(b.Status.Conditions, cType)
			return cond != nil && cond.Status == status, nil
		default:
			return false, errors.Errorf("unexpected event type %q: %v", event.Type, event.Object)
		}
	}
}

// IsBundleResourceCond is a condition that checks conditions of a resource.
// Sometimes it is necessary to await a particular resource condition(s) to happen, not a Bundle condition.
func IsBundleResourceCond(t *testing.T, namespace, name string, resource smith_v1.ResourceName, conds ...*cond_v1.Condition) watch.ConditionFunc {
	return func(event watch.Event) (bool, error) {
		metaObj := event.Object.(meta_v1.Object)
		if metaObj.GetNamespace() != namespace || metaObj.GetName() != name {
			return false, nil
		}
		switch event.Type {
		case watch.Added, watch.Modified:
			b := event.Object.(*smith_v1.Bundle)
			_, resStatus := b.Status.GetResourceStatus(resource)
			if resStatus == nil {
				t.Logf("Resource status for resource %q not found. Bundle status: %s", resource, &b.Status)
				return false, nil
			}
			for _, condExpected := range conds {
				_, condActual := cond_v1.FindCondition(resStatus.Conditions, condExpected.Type)
				if condActual == nil {
					t.Logf("Resource condition %q for resource %q not found", condExpected.Type, resource)
					return false, nil
				}
				if condExpected.Status != condActual.Status ||
					condExpected.Reason != condActual.Reason ||
					condExpected.Message != condActual.Message {
					t.Logf("Resource condition %q for resource %q not satisfied: %s", condExpected.Type, resource, condActual)
					return false, nil
				}
				t.Logf("Resource condition %q for resource %q satisfied", condExpected.Type, resource)
			}
			return true, nil
		default:
			return false, errors.Errorf("unexpected event type %q: %v", event.Type, event.Object)
		}
	}
}

func IsBundleObservedGenerationCond(namespace, name string) watch.ConditionFunc {
	return func(event watch.Event) (bool, error) {
		metaObj := event.Object.(meta_v1.Object)
		if metaObj.GetNamespace() != namespace || metaObj.GetName() != name {
			return false, nil
		}
		b := event.Object.(*smith_v1.Bundle)
		return b.Status.ObservedGeneration >= b.Generation, nil
	}
}

func TestSetup(t *testing.T) (*rest.Config, *kubernetes.Clientset, *smithClientset.Clientset) {
	configFileFrom := os.Getenv("KUBERNETES_CONFIG_FROM")
	configFileName := os.Getenv("KUBERNETES_CONFIG_FILENAME")
	configContext := os.Getenv("KUBERNETES_CONFIG_CONTEXT")

	config, err := client.LoadConfig(configFileFrom, configFileName, configContext)
	require.NoError(t, err)

	mainClient, err := kubernetes.NewForConfig(config)
	require.NoError(t, err)

	smithClient, err := smithClientset.NewForConfig(config)
	require.NoError(t, err)

	return config, mainClient, smithClient
}

func SetupApp(t *testing.T, bundle *smith_v1.Bundle, serviceCatalog, createBundle bool, test TestFunc, args ...interface{}) {
	convertBundleResourcesToUnstrucutred(t, bundle)
	config, mainClient, smithClient := TestSetup(t)
	rm := restmapper.NewDeferredDiscoveryRESTMapper(
		&smart.CachedDiscoveryClient{
			DiscoveryInterface: mainClient.Discovery(),
		},
	)
	dynamicClient, err := dynamic.NewForConfig(config)
	require.NoError(t, err)
	sc := &smart.DynamicClient{
		DynamicClient: dynamicClient,
		RESTMapper:    rm,
	}

	logger := zaptest.NewLogger(t)
	defer logger.Sync() // nolint: errcheck

	cfg := &Config{
		T:           t,
		Logger:      logger,
		Namespace:   fmt.Sprintf("smith-it-%d", rand.Uint32()),
		Bundle:      bundle,
		Config:      config,
		MainClient:  mainClient,
		Sc:          sc,
		SmithClient: smithClient,
	}

	t.Logf("Creating namespace %q", cfg.Namespace)
	_, err = mainClient.CoreV1().Namespaces().Create(&core_v1.Namespace{
		ObjectMeta: meta_v1.ObjectMeta{
			Name: cfg.Namespace,
		},
	})
	require.NoError(t, err)

	defer func() {
		cli := smithClient.SmithV1().Bundles(cfg.Namespace)
		b, err := cli.Get(bundle.Name, meta_v1.GetOptions{})
		if err == nil {
			if len(b.Finalizers) > 0 {
				t.Logf("Removing finalizers from Bundle %q", bundle.Name)
				b.Finalizers = nil
				_, err = cli.Update(b)
				assert.NoError(t, err)
			}
		} else if !api_errors.IsNotFound(err) {
			assert.NoError(t, err) // unexpected error
		}
		t.Logf("Deleting namespace %q", cfg.Namespace)
		assert.NoError(t, mainClient.CoreV1().Namespaces().Delete(cfg.Namespace, nil))
	}()

	stgr := stager.New()
	defer stgr.Shutdown()

	ctxTest, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()

	apiExtClient, err := apiExtClientset.NewForConfig(config)
	require.NoError(t, err)

	crdInf := apiext_v1b1inf.NewCustomResourceDefinitionInformer(apiExtClient, 0, cache.Indexers{})
	stage := stgr.NextStage()
	stage.StartWithChannel(crdInf.Run)

	// We must wait for crdInf to populate its cache to avoid reading from an empty cache
	// in resources.EnsureCrdExistsAndIsEstablished().
	if !cache.WaitForCacheSync(ctxTest.Done(), crdInf.HasSynced) {
		t.Fatal("wait for CRD Informer was cancelled")
	}

	crdLister := apiext_v1b1list.NewCustomResourceDefinitionLister(crdInf.GetIndexer())
	require.NoError(t, resources.EnsureCrdExistsAndIsEstablished(ctxTest, logger, apiExtClient, crdLister, sleeper.Crd()))
	require.NoError(t, resources.EnsureCrdExistsAndIsEstablished(ctxTest, logger, apiExtClient, crdLister, resources.BundleCrd()))

	stage.StartWithContext(func(ctx context.Context) {
		apl := &ctrlApp.App{
			Logger:             logger,
			MainClient:         mainClient,
			PrometheusRegistry: prometheus.NewPedanticRegistry(),
			Name:               "smith",
			RestConfig:         config,
			Controllers: []ctrl.Constructor{
				&app.BundleControllerConstructor{
					ServiceCatalogSupport: serviceCatalog,
				},
			},
		}
		if e := apl.Run(ctx); e != context.Canceled && e != context.DeadlineExceeded {
			assert.NoError(t, e)
		}
	})

	if createBundle {
		res := &smith_v1.Bundle{}
		cfg.CreateObject(ctxTest, bundle, res, smith_v1.BundleResourcePlural, smithClient.SmithV1().RESTClient())
		cfg.CreatedBundle = res
	}

	test(ctxTest, t, cfg, args...)
}

func (cfg *Config) AssertBundle(ctx context.Context, bundle *smith_v1.Bundle) *smith_v1.Bundle {
	bundleRes := cfg.AwaitBundleCondition(
		IsBundleStatusCond(cfg.Namespace, cfg.Bundle.Name, smith_v1.BundleReady, cond_v1.ConditionTrue))

	smith_testing.AssertCondition(cfg.T, bundleRes, smith_v1.BundleReady, cond_v1.ConditionTrue)
	smith_testing.AssertCondition(cfg.T, bundleRes, smith_v1.BundleInProgress, cond_v1.ConditionFalse)
	smith_testing.AssertCondition(cfg.T, bundleRes, smith_v1.BundleError, cond_v1.ConditionFalse)
	if assert.Len(cfg.T, bundleRes.Spec.Resources, len(bundle.Spec.Resources), "%#v", bundleRes) {
		for i, res := range bundle.Spec.Resources {
			spec, err := util.RuntimeToUnstructured(res.Spec.Object)
			if !assert.NoError(cfg.T, err) {
				continue
			}
			actual, err := util.RuntimeToUnstructured(bundleRes.Spec.Resources[i].Spec.Object)
			if !assert.NoError(cfg.T, err) {
				continue
			}
			assert.Equal(cfg.T, spec, actual, "%#v", bundleRes)
			smith_testing.AssertResourceCondition(cfg.T, bundleRes, res.Name, smith_v1.ResourceBlocked, cond_v1.ConditionFalse)
			smith_testing.AssertResourceCondition(cfg.T, bundleRes, res.Name, smith_v1.ResourceInProgress, cond_v1.ConditionFalse)
			smith_testing.AssertResourceCondition(cfg.T, bundleRes, res.Name, smith_v1.ResourceReady, cond_v1.ConditionTrue)
			smith_testing.AssertResourceCondition(cfg.T, bundleRes, res.Name, smith_v1.ResourceError, cond_v1.ConditionFalse)
		}
	}

	return bundleRes
}

func (cfg *Config) AssertBundleTimeout(ctx context.Context, bundle *smith_v1.Bundle) *smith_v1.Bundle {
	ctxTimeout, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return cfg.AssertBundle(ctxTimeout, bundle)
}

func convertBundleResourcesToUnstrucutred(t *testing.T, bundle *smith_v1.Bundle) {
	// Convert all typed objects into unstructured ones
	for i, res := range bundle.Spec.Resources {
		if res.Spec.Object != nil {
			resUnstr, err := util.RuntimeToUnstructured(res.Spec.Object)
			require.NoError(t, err)
			bundle.Spec.Resources[i].Spec.Object = resUnstr
		}
	}
}
