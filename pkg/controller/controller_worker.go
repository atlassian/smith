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
	key, quit := c.Queue.Get()
	if quit {
		return false
	}
	defer c.Queue.Done(key)

	retriable, err := c.processKey(key.(string))
	c.handleErr(retriable, err, key)

	return true
}

func (c *BundleController) handleErr(retriable bool, err error, key interface{}) {
	if err == nil {
		c.Queue.Forget(key)
		return
	}
	if retriable && c.Queue.NumRequeues(key) < maxRetries {
		log.Printf("[WORKER][%s] Error syncing Bundle: %v", key, err)
		c.Queue.AddRateLimited(key)
		return
	}

	log.Printf("[WORKER][%s] Dropping Bundle out of the queue: %v", key, err)
	c.Queue.Forget(key)
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
	bundleObj, exists, err := c.BundleInf.GetIndexer().GetByKey(key)
	if err != nil {
		return false, err
	}
	if !exists {
		log.Printf("[WORKER][%s] Bundle has been deleted", key)
		return false, nil
	}

	st := bundleSyncTask{
		bundleClient: c.BundleClient,
		smartClient:  c.SmartClient,
		rc:           c.Rc,
		store:        c.Store,
		specCheck:    c.SpecCheck,
		bundle:       bundleObj.(*smith_v1.Bundle).DeepCopy(), // Deep-copy otherwise we are mutating our cache.
		plugins:      c.Plugins,
		scheme:       c.Scheme,
	}
	retriable, err := st.process()
	return st.handleProcessResult(retriable, err)
}
