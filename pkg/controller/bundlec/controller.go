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
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	core_v1_client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
)

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
func (c *Controller) Prepare(crdInf cache.SharedIndexInformer, resourceInfs map[schema.GroupVersionKind]cache.SharedIndexInformer) {
	c.crdContext, c.crdContextCancel = context.WithCancel(context.Background())
	crdInf.AddEventHandler(&crdEventHandler{
		controller: c,
		watchers:   make(map[string]watchState),
	})

	for gvk, resourceInf := range resourceInfs {
		resourceHandler := &handlers.ControlledResourceHandler{
			Logger:          c.Logger,
			WorkQueue:       c.WorkQueue,
			ControllerIndex: &controllerIndexAdapter{bundleStore: c.BundleStore},
			ControllerGvk:   smith_v1.BundleGVK,
			Gvk:             gvk,
		}
		resourceInf.AddEventHandler(resourceHandler)
	}
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
		Interface: c.MainClient.CoreV1().Events(""),
	}
	recordingWatch := c.Broadcaster.StartRecordingToSink(&sink)
	defer recordingWatch.Stop()

	c.ReadyForWork()

	<-ctx.Done()
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
