package bundlec

import (
	"context"
	"sync"
	"time"

	"github.com/ash2k/stager/wait"
	"github.com/atlassian/ctrl"
	"github.com/atlassian/ctrl/handlers"
	"github.com/atlassian/ctrl/logz"
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	smithClient_v1 "github.com/atlassian/smith/pkg/client/clientset_generated/clientset/typed/smith/v1"
	"github.com/atlassian/smith/pkg/plugin"
	"github.com/atlassian/smith/pkg/statuschecker"
	"github.com/atlassian/smith/pkg/store"
	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
	apps_v1 "k8s.io/api/apps/v1"
	core_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	core_v1_client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
)

const (
	byConfigMapNamespaceNameIndexName = "ByConfigMap"
	bySecretNamespaceNameIndexName    = "BySecret"
)

type byIndexFunc func(indexName, indexKey string) ([]interface{}, error)
type indexKeyFunc func(namespace, name string) string

type Controller struct {
	// wg.Wait() is called from Run() and first wg.Add() may be called concurrently from CRD listener
	// to start an Informer. This is a data race. This mutex is used to ensure ordering.
	// See https://github.com/atlassian/smith/issues/156
	// See https://github.com/golang/go/blob/fbc8973a6bc88b50509ea738f475b36ef756bf90/src/sync/waitgroup.go#L123-L126
	wgLock   sync.Mutex
	wg       wait.Group
	stopping bool

	crdContext       context.Context
	crdContextCancel context.CancelFunc

	Logger *zap.Logger

	ReadyForWork func()
	MainClient   kubernetes.Interface
	BundleClient smithClient_v1.BundlesGetter
	BundleStore  BundleStore
	SmartClient  SmartClient
	Rc           statuschecker.Interface
	Store        Store
	SpecChecker  SpecChecker
	WorkQueue    ctrl.WorkQueueProducer

	// CRD
	CrdResyncPeriod time.Duration
	Namespace       string

	PluginContainers map[smith_v1.PluginName]plugin.Container
	Scheme           *runtime.Scheme

	Catalog *store.Catalog

	// Metrics
	BundleTransitionCounter         *prometheus.CounterVec
	BundleResourceTransitionCounter *prometheus.CounterVec

	Broadcaster record.EventBroadcaster
	Recorder    record.EventRecorder
}

// Prepare prepares the controller to be run.
func (c *Controller) Prepare(crdInf cache.SharedIndexInformer, resourceInfs map[schema.GroupVersionKind]cache.SharedIndexInformer) error {
	c.crdContext, c.crdContextCancel = context.WithCancel(context.Background())
	crdInf.AddEventHandler(&crdEventHandler{
		controller: c,
		watchers:   make(map[string]watchState),
	})
	deploymentInf := resourceInfs[apps_v1.SchemeGroupVersion.WithKind("Deployment")]
	err := deploymentInf.AddIndexers(cache.Indexers{
		byConfigMapNamespaceNameIndexName: deploymentByConfigMapNamespaceNameIndex,
		bySecretNamespaceNameIndexName:    deploymentBySecretNamespaceNameIndex,
	})
	if err != nil {
		return errors.WithStack(err)
	}
	deploymentByIndex := deploymentInf.GetIndexer().ByIndex
	// ConfigMap -> Deployment -> Bundle event propagation
	configMapGVK := core_v1.SchemeGroupVersion.WithKind("ConfigMap")
	configMapInf := resourceInfs[configMapGVK]
	configMapInf.AddEventHandler(&handlers.LookupHandler{
		Logger:    c.Logger,
		WorkQueue: c.WorkQueue,
		Gvk:       configMapGVK,
		Lookup:    c.lookupBundleByObjectByIndex(deploymentByIndex, byConfigMapNamespaceNameIndexName, byNamespaceNameIndexKey),
	})
	// Secret -> Deployment -> Bundle event propagation
	secretGVK := core_v1.SchemeGroupVersion.WithKind("Secret")
	secretInf := resourceInfs[secretGVK]
	secretInf.AddEventHandler(&handlers.LookupHandler{
		Logger:    c.Logger,
		WorkQueue: c.WorkQueue,
		Gvk:       secretGVK,
		Lookup:    c.lookupBundleByObjectByIndex(deploymentByIndex, bySecretNamespaceNameIndexName, byNamespaceNameIndexKey),
	})
	serviceInstanceInf, ok := resourceInfs[sc_v1b1.SchemeGroupVersion.WithKind("ServiceInstance")]
	if ok { // Service Catalog support is enabled
		// Secret -> ServiceInstance -> Bundle event propagation
		err := serviceInstanceInf.AddIndexers(cache.Indexers{
			bySecretNamespaceNameIndexName: serviceInstanceBySecretNamespaceNameIndex,
		})
		if err != nil {
			return errors.WithStack(err)
		}
		serviceInstanceByIndex := serviceInstanceInf.GetIndexer().ByIndex
		secretInf.AddEventHandler(&handlers.LookupHandler{
			Logger:    c.Logger,
			WorkQueue: c.WorkQueue,
			Gvk:       secretGVK,
			Lookup:    c.lookupBundleByObjectByIndex(serviceInstanceByIndex, bySecretNamespaceNameIndexName, byNamespaceNameIndexKey),
		})
		// Secret -> ServiceBinding -> Bundle event propagation
		serviceBindingInf := resourceInfs[sc_v1b1.SchemeGroupVersion.WithKind("ServiceBinding")]
		err = serviceBindingInf.AddIndexers(cache.Indexers{
			bySecretNamespaceNameIndexName: serviceBindingBySecretNamespaceNameIndex,
		})
		if err != nil {
			return errors.WithStack(err)
		}
		serviceBindingByIndex := serviceBindingInf.GetIndexer().ByIndex
		secretInf.AddEventHandler(&handlers.LookupHandler{
			Logger:    c.Logger,
			WorkQueue: c.WorkQueue,
			Gvk:       secretGVK,
			Lookup:    c.lookupBundleByObjectByIndex(serviceBindingByIndex, bySecretNamespaceNameIndexName, byNamespaceNameIndexKey),
		})
	}
	// Standard handler
	for gvk, resourceInf := range resourceInfs {
		resourceInf.AddEventHandler(&handlers.ControlledResourceHandler{
			Logger:          c.Logger,
			WorkQueue:       c.WorkQueue,
			ControllerIndex: &controllerIndexAdapter{bundleStore: c.BundleStore},
			ControllerGvk:   smith_v1.BundleGVK,
			Gvk:             gvk,
		})
	}
	return nil
}

// Run begins watching and syncing.
// All informers must be synced before this method is invoked.
func (c *Controller) Run(ctx context.Context) {
	defer c.wg.Wait()
	defer c.crdContextCancel() // should be executed after stopping is set to true
	defer func() {
		c.wgLock.Lock()
		defer c.wgLock.Unlock()
		c.stopping = true
	}()

	c.Logger.Info("Starting Bundle controller")
	defer c.Logger.Info("Shutting down Bundle controller")

	sink := core_v1_client.EventSinkImpl{
		Interface: c.MainClient.CoreV1().Events(meta_v1.NamespaceNone),
	}
	recordingWatch := c.Broadcaster.StartRecordingToSink(&sink)
	defer recordingWatch.Stop()

	c.ReadyForWork()

	<-ctx.Done()
}

// lookupBundleByObjectByIndex returns a function that can be used to perform lookups of Bundles that contain
// objects returned from an index.
func (c *Controller) lookupBundleByObjectByIndex(byIndex byIndexFunc, indexName string, indexKey indexKeyFunc) func(runtime.Object) ([]runtime.Object, error) {
	return func(obj runtime.Object) ([]runtime.Object /*bundles*/, error) {
		// obj is an object that is referred by some other object that might be in a Bundle
		objMeta := obj.(meta_v1.Object)
		// find all object that reference this obj
		objsFromIndex, err := byIndex(indexName, indexKey(objMeta.GetNamespace(), objMeta.GetName()))
		if err != nil {
			return nil, err
		}
		var bundles []runtime.Object
		for _, objFromIndex := range objsFromIndex {
			runtimeObjFromIndex := objFromIndex.(runtime.Object)
			metaObjFromIndex := objFromIndex.(meta_v1.Object)
			gvks, _, err := c.Scheme.ObjectKinds(runtimeObjFromIndex)
			if err != nil {
				// Log and continue to try to process other objects if there are any more in objsFromIndex
				// This shouldn't happen normally
				c.Logger.
					With(zap.Error(err), logz.Namespace(metaObjFromIndex), logz.Object(metaObjFromIndex)).
					Sugar().Errorf("Could not determine GVK of an object")
				continue
			}
			gks := make(map[schema.GroupKind]struct{}, len(gvks)) // not clear if duplicates are allowed, so de-dupe
			for _, gvk := range gvks {
				gks[gvk.GroupKind()] = struct{}{}
			}

			// find all Bundles that contain this object
			for gk := range gks {
				bundlesForObject, err := c.BundleStore.GetBundlesByObject(gk, metaObjFromIndex.GetNamespace(), metaObjFromIndex.GetName())
				if err != nil {
					// Log and continue to try to process other GKs
					c.Logger.
						With(zap.Error(err), logz.Namespace(metaObjFromIndex), logz.Object(metaObjFromIndex)).
						Sugar().Errorf("Failed to get Bundles by object")
					continue
				}
				for _, bundle := range bundlesForObject {
					bundles = append(bundles, bundle)
				}
			}
		}
		return bundles, nil
	}
}

type controllerIndexAdapter struct {
	bundleStore BundleStore
}

func (c *controllerIndexAdapter) ControllerByObject(gk schema.GroupKind, namespace, name string) ([]runtime.Object, error) {
	bundles, err := c.bundleStore.GetBundlesByObject(gk, namespace, name)
	if err != nil {
		return nil, err
	}
	objs := make([]runtime.Object, 0, len(bundles))
	for _, bundle := range bundles {
		objs = append(objs, bundle)
	}
	return objs, nil
}

func serviceInstanceBySecretNamespaceNameIndex(obj interface{}) ([]string, error) {
	instance := obj.(*sc_v1b1.ServiceInstance)
	return indexKeysFromParametersFrom(instance.Namespace, instance.Spec.ParametersFrom), nil
}

func serviceBindingBySecretNamespaceNameIndex(obj interface{}) ([]string, error) {
	binding := obj.(*sc_v1b1.ServiceBinding)
	return indexKeysFromParametersFrom(binding.Namespace, binding.Spec.ParametersFrom), nil
}

func indexKeysFromParametersFrom(namespace string, parametersFrom []sc_v1b1.ParametersFromSource) []string {
	var indexKeys []string
	for _, from := range parametersFrom {
		fromValue := from.SecretKeyRef
		if fromValue == nil {
			continue
		}
		indexKeys = append(indexKeys, byNamespaceNameIndexKey(namespace, fromValue.Name))
	}
	return indexKeys
}

func deploymentByConfigMapNamespaceNameIndex(obj interface{}) ([]string, error) {
	d := obj.(*apps_v1.Deployment)
	indexKeys := configMapNamespaceNameIndexKeysForContainers(d.Namespace, d.Spec.Template.Spec.Containers)
	indexKeys = append(indexKeys, configMapNamespaceNameIndexKeysForContainers(d.Namespace, d.Spec.Template.Spec.InitContainers)...)
	return indexKeys, nil
}

func configMapNamespaceNameIndexKeysForContainers(namespace string, containers []core_v1.Container) []string {
	var indexKeys []string
	for _, container := range containers {
		for _, envFrom := range container.EnvFrom {
			configMapRef := envFrom.ConfigMapRef
			if configMapRef == nil {
				continue
			}
			indexKeys = append(indexKeys, byNamespaceNameIndexKey(namespace, configMapRef.Name))
		}
		for _, env := range container.Env {
			valueFrom := env.ValueFrom
			if valueFrom == nil {
				continue
			}

			configMapKeyRef := valueFrom.ConfigMapKeyRef
			if configMapKeyRef == nil {
				continue
			}
			indexKeys = append(indexKeys, byNamespaceNameIndexKey(namespace, configMapKeyRef.Name))
		}
	}
	return indexKeys
}

func deploymentBySecretNamespaceNameIndex(obj interface{}) ([]string, error) {
	d := obj.(*apps_v1.Deployment)
	indexKeys := secretNamespaceNameIndexKeysForContainers(d.Namespace, d.Spec.Template.Spec.Containers)
	indexKeys = append(indexKeys, secretNamespaceNameIndexKeysForContainers(d.Namespace, d.Spec.Template.Spec.InitContainers)...)
	return indexKeys, nil
}

func secretNamespaceNameIndexKeysForContainers(namespace string, containers []core_v1.Container) []string {
	var indexKeys []string
	for _, container := range containers {
		for _, envFrom := range container.EnvFrom {
			secretRef := envFrom.SecretRef
			if secretRef == nil {
				continue
			}
			indexKeys = append(indexKeys, byNamespaceNameIndexKey(namespace, secretRef.Name))
		}
		for _, env := range container.Env {
			valueFrom := env.ValueFrom
			if valueFrom == nil {
				continue
			}

			secretKeyRef := valueFrom.SecretKeyRef
			if secretKeyRef == nil {
				continue
			}
			indexKeys = append(indexKeys, byNamespaceNameIndexKey(namespace, secretKeyRef.Name))
		}
	}
	return indexKeys
}

func byNamespaceNameIndexKey(namespace, name string) string {
	return namespace + "/" + name
}
