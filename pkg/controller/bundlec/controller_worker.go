package bundlec

import (
	"sort"

	"github.com/atlassian/ctrl"
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	api_errors "k8s.io/apimachinery/pkg/api/errors"
)

func (c *Controller) Process(pctx *ctrl.ProcessContext) (bool /*external*/, bool /*retriable*/, error) {
	return c.ProcessBundle(pctx.Logger, pctx.Object.(*smith_v1.Bundle))
}

// ProcessBundle is only visible for testing purposes. Should not be called directly.
func (c *Controller) ProcessBundle(logger *zap.Logger, bundle *smith_v1.Bundle) (bool /*external*/, bool /*retriable*/, error) {
	st := bundleSyncTask{
		logger:                          logger,
		bundleClient:                    c.BundleClient,
		smartClient:                     c.SmartClient,
		checker:                         c.Rc,
		store:                           c.Store,
		specChecker:                     c.SpecChecker,
		bundle:                          bundle,
		pluginContainers:                c.PluginContainers,
		scheme:                          c.Scheme,
		catalog:                         c.Catalog,
		bundleTransitionCounter:         c.BundleTransitionCounter,
		bundleResourceTransitionCounter: c.BundleResourceTransitionCounter,
		recorder:                        c.Recorder,
	}

	var external bool
	var retriable bool
	var err error
	if st.bundle.DeletionTimestamp != nil {
		external, retriable, err = st.processDeleted()
	} else {
		external, retriable, err = st.processNormal()
	}
	if err != nil {
		cause := errors.Cause(err)
		// short circuit on conflicts
		if api_errors.IsConflict(cause) {
			return external, retriable, err
		}
		// proceed to handleProcessResult() for all other errors
	}

	// Updates bundle status
	handleProcessRetriable, handleProcessErr := st.handleProcessResult(retriable, err)

	// Inspect the resources for failures. They can fail for many different reasons.
	// The priority of errors to bubble up to the ctrl layer are:
	//  1. processDeleted/processNormal errors
	//  2. Internal resource processing errors are raised first
	//  3. External resource processing errors are raised last
	//  4. handleProcessResult errors of any sort.

	// Handle the errors from processDeleted/processNormal, taking precedence
	// over the handleProcessErr if any.
	if err != nil {
		if handleProcessErr != nil {
			st.logger.Error("Error processing Bundle", zap.Error(handleProcessErr))
		}

		return external, retriable || handleProcessRetriable, err
	}

	// Inspect resources, returning an error if necessary
	allExternalErrors := true
	hasRetriableResourceErr := false
	var failedResources []string
	for resName, resInfo := range st.processedResources {
		resErr := resInfo.fetchError()

		if resErr != nil {
			allExternalErrors = allExternalErrors && resErr.isExternalError
			hasRetriableResourceErr = hasRetriableResourceErr || resErr.isRetriableError
			failedResources = append(failedResources, string(resName))
		}
	}
	if len(failedResources) > 0 {
		if handleProcessErr != nil {
			st.logger.Error("Error processing Bundle", zap.Error(handleProcessErr))
		}
		// stable output
		sort.Strings(failedResources)
		err := errors.Errorf("error processing resource(s): %q", failedResources)
		return allExternalErrors, hasRetriableResourceErr || handleProcessRetriable, err
	}

	// Otherwise, return the result from handleProcessResult
	return false, handleProcessRetriable, handleProcessErr
}
