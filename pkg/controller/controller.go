package controller

import (
	"context"
	"log"
	"time"

	"github.com/atlassian/smith"
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	smithClient_v1 "github.com/atlassian/smith/pkg/client/clientset_generated/clientset/typed/smith/v1"

	"github.com/ash2k/stager/wait"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

const (
	// maxRetries is the number of times a Bundle object will be retried before it is dropped out of the queue.
	// With the current rate-limiter in use (5ms*2^(maxRetries-1)) the following numbers represent the times
	// a Bundle is going to be requeued:
	//
	// 5ms, 10ms, 20ms, 40ms, 80ms, 160ms, 320ms, 640ms, 1.3s, 2.6s, 5.1s, 10.2s, 20.4s, 41s, 82s
	maxRetries = 15
	// Work queue deduplicates scheduled keys. This is the period it waits for duplicate keys before letting the work
	// to be dequeued.
	workDeduplicationPeriod = 50 * time.Millisecond
)

type BundleController struct {
	wg           wait.Group
	bundleInf    cache.SharedIndexInformer
	crdInf       cache.SharedIndexInformer
	bundleClient smithClient_v1.BundlesGetter
	bundleStore  BundleStore
	smartClient  smith.SmartClient
	rc           ReadyChecker
	store        Store
	specCheck    SpecCheck
	// Bundle objects that need to be synced.
	queue   workqueue.RateLimitingInterface
	workers int

	// CRD
	crdResyncPeriod time.Duration
	resourceHandler cache.ResourceEventHandler
}

func New(bundleInf, crdInf cache.SharedIndexInformer, bundleClient smithClient_v1.BundlesGetter, bundleStore BundleStore,
	sc smith.SmartClient, rc ReadyChecker, store Store, specCheck SpecCheck, queue workqueue.RateLimitingInterface,
	workers int, crdResyncPeriod time.Duration, resourceInfs map[schema.GroupVersionKind]cache.SharedIndexInformer) *BundleController {
	c := &BundleController{
		bundleInf:       bundleInf,
		crdInf:          crdInf,
		bundleClient:    bundleClient,
		bundleStore:     bundleStore,
		smartClient:     sc,
		rc:              rc,
		store:           store,
		specCheck:       specCheck,
		queue:           queue,
		workers:         workers,
		crdResyncPeriod: crdResyncPeriod,
	}
	bundleInf.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.onBundleAdd,
		UpdateFunc: c.onBundleUpdate,
		DeleteFunc: c.onBundleDelete,
	})
	c.resourceHandler = cache.ResourceEventHandlerFuncs{
		AddFunc:    c.onResourceAdd,
		UpdateFunc: c.onResourceUpdate,
		DeleteFunc: c.onResourceDelete,
	}
	for _, resourceInf := range resourceInfs {
		resourceInf.AddEventHandler(c.resourceHandler)
	}
	return c
}

// Run begins watching and syncing.
func (c *BundleController) Run(ctx context.Context) {
	defer c.wg.Wait()
	defer c.queue.ShutDown()

	log.Print("Starting Bundle controller")
	defer log.Print("Shutting down Bundle controller")

	c.crdInf.AddEventHandler(&crdEventHandler{
		ctx:              ctx,
		BundleController: c,
		watchers:         make(map[string]watchState),
	})

	if !cache.WaitForCacheSync(ctx.Done(), c.bundleInf.HasSynced) {
		return
	}

	for i := 0; i < c.workers; i++ {
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
	c.queue.AddAfter(key, workDeduplicationPeriod)
}
