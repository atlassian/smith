package bundlec

import (
	"context"
	"sync"
	"time"

	"github.com/ash2k/stager/wait"
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	smithClient_v1 "github.com/atlassian/smith/pkg/client/clientset_generated/clientset/typed/smith/v1"
	"github.com/atlassian/smith/pkg/plugin"
	"github.com/atlassian/smith/pkg/store"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

const (
	// Work queue deduplicates scheduled keys. This is the period it waits for duplicate keys before letting the work
	// to be dequeued.
	workDeduplicationPeriod = 50 * time.Millisecond
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

	BundleInf    cache.SharedIndexInformer
	BundleClient smithClient_v1.BundlesGetter
	BundleStore  BundleStore
	SmartClient  SmartClient
	Rc           ReadyChecker
	Store        Store
	SpecCheck    SpecCheck
	// Bundle objects that need to be synced.
	Queue   workqueue.RateLimitingInterface
	Workers int

	// CRD
	CrdResyncPeriod time.Duration
	resourceHandler cache.ResourceEventHandler
	Namespace       string

	PluginContainers map[smith_v1.PluginName]plugin.PluginContainer
	Scheme           *runtime.Scheme

	Catalog *store.Catalog
}

// Prepare prepares the controller to be run.
func (c *Controller) Prepare(crdInf cache.SharedIndexInformer, resourceInfs map[schema.GroupVersionKind]cache.SharedIndexInformer) {
	c.crdContext, c.crdContextCancel = context.WithCancel(context.Background())
	c.resourceHandler = cache.ResourceEventHandlerFuncs{
		AddFunc:    c.onResourceAdd,
		UpdateFunc: c.onResourceUpdate,
		DeleteFunc: c.onResourceDelete,
	}
	c.BundleInf.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.onBundleAdd,
		UpdateFunc: c.onBundleUpdate,
		DeleteFunc: c.onBundleDelete,
	})
	crdInf.AddEventHandler(&crdEventHandler{
		Controller: c,
		watchers:   make(map[string]watchState),
	})

	for _, resourceInf := range resourceInfs {
		resourceInf.AddEventHandler(c.resourceHandler)
	}
}

// Run begins watching and syncing.
// All informers must be synched before this method is invoked.
func (c *Controller) Run(ctx context.Context) {
	defer c.wg.Wait()
	defer c.crdContextCancel() // should be executed after stopping is set to true
	defer func() {
		c.wgLock.Lock()
		defer c.wgLock.Unlock()
		c.stopping = true
	}()
	defer c.Queue.ShutDown()

	c.Logger.Info("Starting Bundle controller")
	defer c.Logger.Info("Shutting down Bundle controller")

	for i := 0; i < c.Workers; i++ {
		c.wg.Start(c.worker)
	}

	<-ctx.Done()
}

func (c *Controller) enqueue(bundle *smith_v1.Bundle) {
	c.enqueueKey(queueKey{
		namespace: bundle.Namespace,
		name:      bundle.Name,
	})
}

func (c *Controller) enqueueKey(key queueKey) {
	c.Queue.AddAfter(key, workDeduplicationPeriod)
}
