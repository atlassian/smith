package bundlec

import (
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/smith/pkg/controller"
	"github.com/atlassian/smith/pkg/util/logz"
	"go.uber.org/zap"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func (c *Controller) Process(pctx *controller.ProcessContext) (retriableRet bool, errRet error) {
	return c.ProcessBundle(pctx.Logger, pctx.Object.(*smith_v1.Bundle))
}

// ProcessBundle is only visible for testing purposes. Should not be called directly.
func (c *Controller) ProcessBundle(logger *zap.Logger, bundle *smith_v1.Bundle) (retriableRet bool, errRet error) {
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
