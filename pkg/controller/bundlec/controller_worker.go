package bundlec

import (
	"github.com/atlassian/ctrl"
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"go.uber.org/zap"
)

func (c *Controller) Process(pctx *ctrl.ProcessContext) (retriableRet bool, errRet error) {
	return c.ProcessBundle(pctx.Logger, pctx.Object.(*smith_v1.Bundle))
}

// ProcessBundle is only visible for testing purposes. Should not be called directly.
func (c *Controller) ProcessBundle(logger *zap.Logger, bundle *smith_v1.Bundle) (retriableRet bool, errRet error) {
	st := bundleSyncTask{
		logger:                          logger,
		bundleClient:                    c.BundleClient,
		smartClient:                     c.SmartClient,
		rc:                              c.Rc,
		store:                           c.Store,
		specCheck:                       c.SpecCheck,
		bundle:                          bundle,
		pluginContainers:                c.PluginContainers,
		scheme:                          c.Scheme,
		catalog:                         c.Catalog,
		bundleTransitionCounter:         c.BundleTransitionCounter,
		bundleResourceTransitionCounter: c.BundleResourceTransitionCounter,
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
