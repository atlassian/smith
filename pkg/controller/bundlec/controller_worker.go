package bundlec

import (
	"time"

	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/smith/pkg/util/logz"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	api_errors "k8s.io/apimachinery/pkg/api/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	// maxRetries is the number of times a Bundle object will be retried before it is dropped out of the queue.
	// With the current rate-limiter in use (5ms*2^(maxRetries-1)) the following numbers represent the times
	// a Bundle is going to be requeued:
	//
	// 5ms, 10ms, 20ms, 40ms, 80ms, 160ms, 320ms, 640ms, 1.3s, 2.6s, 5.1s, 10.2s, 20.4s, 41s, 82s
	maxRetries = 15
)

type objectRef struct {
	schema.GroupVersionKind
	Name string
}

func (c *Controller) worker() {
	for c.processNextWorkItem() {
	}
}

func (c *Controller) processNextWorkItem() bool {
	key, quit := c.Queue.Get()
	if quit {
		return false
	}
	defer c.Queue.Done(key)

	k := key.(queueKey)

	logger := c.Logger.With(logz.NamespaceName(k.namespace), logz.BundleName(k.name))

	retriable, err := c.processKey(logger, k)
	c.handleErr(logger, retriable, err, k)

	return true
}

func (c *Controller) handleErr(logger *zap.Logger, retriable bool, err error, key queueKey) {
	if err == nil {
		c.Queue.Forget(key)
		return
	}
	if retriable && c.Queue.NumRequeues(key) < maxRetries {
		logger.Info("Error syncing Bundle", zap.Error(err))
		c.Queue.AddRateLimited(key)
		return
	}

	logger.Info("Dropping Bundle out of the queue", zap.Error(err))
	c.Queue.Forget(key)
}

func (c *Controller) processKey(logger *zap.Logger, key queueKey) (retriableRet bool, errRet error) {
	bundleObj, exists, err := c.BundleInf.GetIndexer().GetByKey(key.namespace + "/" + key.name)
	if err != nil {
		return false, errors.Wrapf(err, "failed to get Bundle by key %q", key)
	}
	if !exists {
		logger.Info("Bundle not in cache. Was deleted?")
		return false, nil
	}
	// Deep-copy otherwise we are mutating our cache.
	return c.ProcessBundle(logger, bundleObj.(*smith_v1.Bundle).DeepCopy())
}

// ProcessBundle is only visible for testing purposes. Should not be called directly.
func (c *Controller) ProcessBundle(logger *zap.Logger, bundle *smith_v1.Bundle) (retriableRet bool, errRet error) {
	startTime := time.Now()
	logger.Info("Started syncing Bundle")
	defer func() {
		msg := ""
		if errRet != nil && api_errors.IsConflict(errors.Cause(errRet)) {
			msg = " (conflict)"
			errRet = nil
		}
		logger.Sugar().Infof("Synced Bundle in %v%s", time.Since(startTime), msg)
	}()

	st := bundleSyncTask{
		logger:           logger,
		bundleClient:     c.BundleClient,
		smartClient:      c.SmartClient,
		rc:               c.Rc,
		store:            c.Store,
		specCheck:        c.SpecCheck,
		bundle:           bundle,
		pluginContainers: c.PluginContainers,
		scheme:           c.Scheme,
		catalog:          c.Catalog,
	}

	var retriable bool
	var err error
	if st.bundle.DeletionTimestamp != nil {
		retriable, err = st.processDeleted()
	} else {
		retriable, err = st.processNormal()
	}
	return st.handleProcessResult(retriable, err)
}

func (c *Controller) loggerForObj(obj interface{}) *zap.Logger {
	logger := c.Logger
	metaObj, ok := obj.(meta_v1.Object)
	if ok {
		logger = logger.With(logz.Namespace(metaObj), logz.Object(metaObj))
	}
	runtimeObj, ok := metaObj.(runtime.Object)
	if ok {
		gvk := runtimeObj.GetObjectKind().GroupVersionKind()
		if gvk.Kind == "" || gvk.Version == "" {
			gvks, _, err := c.Scheme.ObjectKinds(runtimeObj)
			if err != nil {
				if !runtime.IsNotRegisteredError(err) {
					logger.With(zap.Error(err)).Sugar().Warnf("Cannot get object's GVK. Type %T", runtimeObj)
				}
				return logger
			}
			gvk = gvks[0]
		}
		logger = logger.With(logz.Gvk(gvk))
	}
	return logger
}
