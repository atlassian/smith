package templatecontroller

import (
	"time"

	"github.com/atlassian/smith/pkg/util/logz"

	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/pkg/errors"
	"github.com/taskcluster/json-e"
	"go.uber.org/zap"
	api_errors "k8s.io/apimachinery/pkg/api/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
)

const (
	// maxRetries is the number of times a TemplateRender object will be retried before it is dropped out of the queue.
	// With the current rate-limiter in use (5ms*2^(maxRetries-1)) the following numbers represent the times
	// a TemplateRender is going to be requeued:
	//
	// 5ms, 10ms, 20ms, 40ms, 80ms, 160ms, 320ms, 640ms, 1.3s, 2.6s, 5.1s, 10.2s, 20.4s, 41s, 82s
	maxRetries = 15
)

func (c *TemplateController) worker() {
	for c.processNextWorkItem() {
	}
}

func (c *TemplateController) processNextWorkItem() bool {
	key, quit := c.Queue.Get()
	if quit {
		return false
	}
	defer c.Queue.Done(key)

	k := key.(string)

	namespace, name, err := cache.SplitMetaNamespaceKey(k)
	if err != nil {
		c.Logger.
			With(zap.Error(err)).
			Sugar().Errorf("Failed to split key %q", key)
		c.Queue.Forget(key)
		return true
	}

	logger := c.Logger.With(logz.NamespaceName(namespace), logz.TemplateRenderName(name))

	retriable, err := c.ProcessKey(logger, k)
	c.handleErr(logger, retriable, err, k)

	return true
}

func (c *TemplateController) handleErr(logger *zap.Logger, retriable bool, err error, key string) {
	if err == nil {
		c.Queue.Forget(key)
		return
	}
	if retriable && c.Queue.NumRequeues(key) < maxRetries {
		logger.Info("Error syncing TemplateRender", zap.Error(err))
		c.Queue.AddRateLimited(key)
		return
	}

	logger.Info("Dropping TemplateRender out of the queue", zap.Error(err))
	c.Queue.Forget(key)
}

// ProcessKey is only visible for testing purposes. Should not be called directly.
func (c *TemplateController) ProcessKey(logger *zap.Logger, key string) (retriableRet bool, errRet error) {
	startTime := time.Now()
	logger.Info("Started syncing TemplateRender")
	defer func() {
		msg := ""
		if errRet != nil && api_errors.IsConflict(errors.Cause(errRet)) {
			msg = " (conflict)"
			errRet = nil
		}
		logger.Sugar().Infof("Synced TemplateRender in %v%s", time.Since(startTime), msg)
	}()
	templateRenderObj, exists, err := c.TemplateRenderInf.GetIndexer().GetByKey(key)
	if err != nil {
		return false, errors.Wrapf(err, "failed to get Template by key %q", key)
	}
	if !exists {
		logger.Info("Template not in cache. Was deleted?")
		return false, nil
	}
	templateRender := templateRenderObj.(*smith_v1.TemplateRender)
	template := map[string]interface{}{
		"foo": "foo",
	}
	result, err := jsone.Render(template, templateRender.Spec.Context)
	if err != nil {
		return false, errors.Wrapf(err, "failed to render template %s", templateRender.Name)
	}
	logger.Sugar().Infof("%+v", result)

	return false, errors.Errorf("TemplateRender processing not implemented: %+v", templateRenderObj)
}

func (c *TemplateController) loggerForObj(obj interface{}) *zap.Logger {
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
