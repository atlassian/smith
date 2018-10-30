package bundlec

import (
	"context"

	"github.com/atlassian/ctrl"
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	api_errors "k8s.io/apimachinery/pkg/api/errors"
)

func (c *Controller) Process(pctx *ctrl.ProcessContext) (bool /*retriable*/, error) {
	return c.ProcessBundle(pctx.Logger, pctx.Object.(*smith_v1.Bundle))
}

// ProcessBundle is only visible for testing purposes. Should not be called directly.
func (c *Controller) ProcessBundle(logger *zap.Logger, bundle *smith_v1.Bundle) (bool /*retriable*/, error) {
	st := bundleSyncTask{
		logger:                          logger,
		bundleClient:                    c.BundleClient,
		smartClient:                     c.SmartClient,
		checker:                         c.Rc,
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
	if err != nil {
		cause := errors.Cause(err)
		if api_errors.IsConflict(cause) || cause == context.Canceled || cause == context.DeadlineExceeded {
			return retriable, err
		}
		// proceed to handleProcessResult() for all other errors
	}
	return st.handleProcessResult(retriable, err)
}
