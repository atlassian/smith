package bundlec

import (
	"context"
	"sync"
	"time"

	"github.com/ash2k/stager/wait"
	"github.com/atlassian/ctrl"
	"github.com/atlassian/ctrl/handlers"
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	smithClient_v1 "github.com/atlassian/smith/pkg/client/clientset_generated/clientset/typed/smith/v1"
	"github.com/atlassian/smith/pkg/plugin"
	"github.com/atlassian/smith/pkg/statuschecker"
	"github.com/atlassian/smith/pkg/store"
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
		byConfigMapNamespaceNameIndexName: byConfigMapNamespaceNameIndex,
		bySecretNamespaceNameIndexName:    bySecretNamespaceNameIndex,
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
		Lookup:    c.lookupBundleByDeploymentByIndex(deploymentByIndex, byConfigMapNamespaceNameIndexName, byConfigMapNamespaceNameIndexKey),
	})
	// Secret -> Deployment -> Bundle event propagation
	secretGVK := core_v1.SchemeGroupVersion.WithKind("Secret")
	secretInf := resourceInfs[secretGVK]
	secretInf.AddEventHandler(&handlers.LookupHandler{
		Logger:    c.Logger,
		WorkQueue: c.WorkQueue,
		Gvk:       secretGVK,
		Lookup:    c.lookupBundleByDeploymentByIndex(deploymentByIndex, bySecretNamespaceNameIndexName, bySecretNamespaceNameIndexKey),
	})
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

// lookupBundleByDeploymentByIndex returns a function that can be used to perform lookups of Bundles that contain
// Deployment objects that reference ConfigMap/Secret objects.
func (c *Controller) lookupBundleByDeploymentByIndex(byIndex byIndexFunc, indexName string, indexKey indexKeyFunc) func(runtime.Object) ([]runtime.Object, error) {
	deploymentGK := schema.GroupKind{
		Group: apps_v1.GroupName,
		Kind:  "Deployment",
	}
	return func(obj runtime.Object) ([]runtime.Object /*bundles*/, error) {
		// obj is ConfigMap or Secret
		objMeta := obj.(meta_v1.Object)
		// find all Deployments that reference this obj
		deploymentsFromIndex, err := byIndex(indexName, indexKey(objMeta.GetNamespace(), objMeta.GetName()))
		if err != nil {
			return nil, err
		}
		var bundles []runtime.Object
		for _, deploymentInterface := range deploymentsFromIndex {
			deployment := deploymentInterface.(*apps_v1.Deployment)
			// find all Bundles that reference this Deployment
			bundlesForDeployment, err := c.BundleStore.GetBundlesByObject(deploymentGK, deployment.Namespace, deployment.Name)
			if err != nil {
				return nil, err
			}
			for _, bundle := range bundlesForDeployment {
				bundles = append(bundles, bundle)
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

func byConfigMapNamespaceNameIndex(obj interface{}) ([]string, error) {
	d := obj.(*apps_v1.Deployment)
	index := configMapNamespaceNameIndexKeysForContainers(d.Namespace, d.Spec.Template.Spec.Containers)
	index = append(index, configMapNamespaceNameIndexKeysForContainers(d.Namespace, d.Spec.Template.Spec.InitContainers)...)
	return index, nil
}

func configMapNamespaceNameIndexKeysForContainers(namespace string, containers []core_v1.Container) []string {
	var indexKeys []string
	for _, container := range containers {
		for _, envFrom := range container.EnvFrom {
			configMapRef := envFrom.ConfigMapRef
			if configMapRef == nil {
				continue
			}
			indexKeys = append(indexKeys, byConfigMapNamespaceNameIndexKey(namespace, configMapRef.Name))
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
			indexKeys = append(indexKeys, byConfigMapNamespaceNameIndexKey(namespace, configMapKeyRef.Name))
		}
	}
	return indexKeys
}

func byConfigMapNamespaceNameIndexKey(configMapNamespace, configMapName string) string {
	return configMapNamespace + "/" + configMapName
}

func bySecretNamespaceNameIndex(obj interface{}) ([]string, error) {
	d := obj.(*apps_v1.Deployment)
	index := secretNamespaceNameIndexKeysForContainers(d.Namespace, d.Spec.Template.Spec.Containers)
	index = append(index, secretNamespaceNameIndexKeysForContainers(d.Namespace, d.Spec.Template.Spec.InitContainers)...)
	return index, nil
}

func secretNamespaceNameIndexKeysForContainers(namespace string, containers []core_v1.Container) []string {
	var indexKeys []string
	for _, container := range containers {
		for _, envFrom := range container.EnvFrom {
			secretRef := envFrom.SecretRef
			if secretRef == nil {
				continue
			}
			indexKeys = append(indexKeys, bySecretNamespaceNameIndexKey(namespace, secretRef.Name))
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
			indexKeys = append(indexKeys, bySecretNamespaceNameIndexKey(namespace, secretKeyRef.Name))
		}
	}
	return indexKeys
}

func bySecretNamespaceNameIndexKey(secretNamespace, secretName string) string {
	return secretNamespace + "/" + secretName
}
