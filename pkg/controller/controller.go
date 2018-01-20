package controller

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/atlassian/smith"
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	smithClient_v1 "github.com/atlassian/smith/pkg/client/clientset_generated/clientset/typed/smith/v1"
	"github.com/atlassian/smith/pkg/plugin"

	"github.com/ash2k/stager/wait"
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

type BundleController struct {
	// wg.Wait() is called from Run() and first wg.Add() may be called concurrently from CRD listener
	// to start an Informer. This is a data race. This mutex is used to ensure ordering.
	// See https://github.com/atlassian/smith/issues/156
	// See https://github.com/golang/go/blob/fbc8973a6bc88b50509ea738f475b36ef756bf90/src/sync/waitgroup.go#L123-L126
	wgLock   sync.Mutex
	wg       wait.Group
	stopping bool

	BundleInf    cache.SharedIndexInformer
	BundleClient smithClient_v1.BundlesGetter
	BundleStore  BundleStore
	SmartClient  smith.SmartClient
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
}

// Prepare prepares the controller to be run.
// ctx must be the same context as the one passed to Run() method.
func (c *BundleController) Prepare(ctx context.Context, crdInf cache.SharedIndexInformer, resourceInfs map[schema.GroupVersionKind]cache.SharedIndexInformer) {
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
		ctx:              ctx,
		BundleController: c,
		watchers:         make(map[string]watchState),
	})

	for _, resourceInf := range resourceInfs {
		resourceInf.AddEventHandler(c.resourceHandler)
	}
}

// Run begins watching and syncing.
// All informers must be synched before this method is invoked.
func (c *BundleController) Run(ctx context.Context) {
	defer c.wg.Wait()
	defer func() {
		c.wgLock.Lock()
		defer c.wgLock.Unlock()
		c.stopping = true
	}()
	defer c.Queue.ShutDown()

	log.Print("Starting Bundle controller")
	defer log.Print("Shutting down Bundle controller")

	for i := 0; i < c.Workers; i++ {
		c.wg.Start(c.worker)
	}

	<-ctx.Done()
}

func (c *BundleController) enqueue(bundle *smith_v1.Bundle) {
	key, err := cache.MetaNamespaceKeyFunc(bundle)
	if err != nil {
		log.Printf("Couldn't get key for Bundle %+v: %v", bundle, err)
		return
	}
	c.enqueueKey(key)
}

func (c *BundleController) enqueueKey(key string) {
	c.Queue.AddAfter(key, workDeduplicationPeriod)
}
