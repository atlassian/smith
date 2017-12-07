package controller

import (
	"log"
	"time"

	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"

	"github.com/pkg/errors"
	api_errors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type objectRef struct {
	schema.GroupVersionKind
	Name string
}

func (c *BundleController) worker() {
	for c.processNextWorkItem() {
	}
}

func (c *BundleController) processNextWorkItem() bool {
	key, quit := c.queue.Get()
	if quit {
		return false
	}
	defer c.queue.Done(key)

	retriable, err := c.processKey(key.(string))
	c.handleErr(retriable, err, key)

	return true
}

func (c *BundleController) handleErr(retriable bool, err error, key interface{}) {
	if err == nil {
		c.queue.Forget(key)
		return
	}
	if retriable && c.queue.NumRequeues(key) < maxRetries {
		log.Printf("[WORKER][%s] Error syncing Bundle: %v", key, err)
		c.queue.AddRateLimited(key)
		return
	}

	log.Printf("[WORKER][%s] Dropping Bundle out of the queue: %v", key, err)
	c.queue.Forget(key)
}

func (c *BundleController) processKey(key string) (retriableRet bool, errRet error) {
	startTime := time.Now()
	log.Printf("[WORKER][%s] Started syncing Bundle", key)
	defer func() {
		msg := ""
		if errRet != nil && api_errors.IsConflict(errors.Cause(errRet)) {
			msg = " (conflict)"
			errRet = nil
		}
		log.Printf("[WORKER][%s] Synced Bundle in %v%s", key, time.Since(startTime), msg)
	}()
	bundleObj, exists, err := c.bundleInf.GetIndexer().GetByKey(key)
	if err != nil {
		return false, err
	}
	if !exists {
		log.Printf("[WORKER][%s] Bundle has been deleted", key)
		return false, nil
	}

	st := syncTask{
		bundleClient: c.bundleClient,
		smartClient:  c.smartClient,
		rc:           c.rc,
		store:        c.store,
		specCheck:    c.specCheck,
		bundle:       bundleObj.(*smith_v1.Bundle).DeepCopy(), // Deep-copy otherwise we are mutating our cache.
		plugins:      c.plugins,
		scheme:       c.scheme,
	}
	retriable, err := st.process()
	return st.handleProcessResult(retriable, err)
}
