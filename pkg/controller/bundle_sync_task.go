package controller

import (
	"context"
	"fmt"

	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	smithClient_v1 "github.com/atlassian/smith/pkg/client/clientset_generated/clientset/typed/smith/v1"
	"github.com/atlassian/smith/pkg/plugin"
	"github.com/atlassian/smith/pkg/store"
	"github.com/atlassian/smith/pkg/util/graph"
	"github.com/atlassian/smith/pkg/util/logz"

	"github.com/pkg/errors"
	"go.uber.org/zap"
	api_errors "k8s.io/apimachinery/pkg/api/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

type bundleSyncTask struct {
	logger             *zap.Logger
	bundleClient       smithClient_v1.BundlesGetter
	smartClient        SmartClient
	rc                 ReadyChecker
	store              Store
	specCheck          SpecCheck
	bundle             *smith_v1.Bundle
	processedResources map[smith_v1.ResourceName]*resourceInfo
	newFinalizers      []string
	pluginContainers   map[smith_v1.PluginName]plugin.PluginContainer
	scheme             *runtime.Scheme
	catalog            *store.Catalog
}

// Parse bundle, build resource graph, traverse graph, assert each resource exists.
// For each resource ensure its dependencies (if any) are in READY state before creating it.
// If at least one dependency is not READY - skip the resource. Rebuild will/should be called once the dependency
// updates it's state (noticed via watching).

// READY state might mean something different for each resource type. For a Custom Resource it may mean
// that a field "State" in the Status of the resource is set to "Ready". It is customizable via
// annotations with some defaults.
func (st *bundleSyncTask) processNormal() (retriableError bool, e error) {
	// If the "deleteResources" finalizer is missing, add it and finish the processing iteration
	if !hasDeleteResourcesFinalizer(st.bundle) {
		st.newFinalizers = addDeleteResourcesFinalizer(st.bundle.GetFinalizers())
		return false, nil
	}

	// Build resource map by name
	resourceMap := make(map[smith_v1.ResourceName]smith_v1.Resource, len(st.bundle.Spec.Resources))
	for _, res := range st.bundle.Spec.Resources {
		if _, exist := resourceMap[res.Name]; exist {
			return false, errors.Errorf("bundle contains two resources with the same name %q", res.Name)
		}
		resourceMap[res.Name] = res
	}

	// Build the graph and topologically sort it
	_, sorted, sortErr := sortBundle(st.bundle)
	if sortErr != nil {
		return false, errors.Wrap(sortErr, "topological sort of resources failed")
	}

	st.processedResources = make(map[smith_v1.ResourceName]*resourceInfo, len(st.bundle.Spec.Resources))

	// Visit vertices in sorted order
	for _, resName := range sorted {
		// Process the resource
		resourceName := resName.(smith_v1.ResourceName)
		logger := st.logger.With(logz.Resource(resourceName))
		res := resourceMap[resourceName]
		rst := resourceSyncTask{
			logger:             logger,
			smartClient:        st.smartClient,
			rc:                 st.rc,
			store:              st.store,
			specCheck:          st.specCheck,
			bundle:             st.bundle,
			processedResources: st.processedResources,
			pluginContainers:   st.pluginContainers,
			scheme:             st.scheme,
			catalog:            st.catalog,
		}
		resInfo := rst.processResource(&res)
		if retriable, err := resInfo.fetchError(); err != nil && api_errors.IsConflict(errors.Cause(err)) {
			// Short circuit on conflict
			return retriable, err
		}
		_, resErr := resInfo.fetchError()
		if resErr != nil {
			logger.Error("Done processing resource", zap.Bool("ready", resInfo.isReady()), zap.Error(resErr))
		} else {
			logger.Info("Done processing resource", zap.Bool("ready", resInfo.isReady()))
		}
		st.processedResources[resourceName] = &resInfo
	}
	if st.isBundleReady() {
		// Delete objects which were removed from the bundle
		retriable, err := st.deleteRemovedResources()
		if err != nil {
			return retriable, err
		}
	}

	return false, nil
}

// Process the bundle marked with DeletionTimestamp
// TODO: remove this method after https://github.com/kubernetes/kubernetes/issues/59850 is fixed
func (st *bundleSyncTask) processDeleted() (retriableError bool, e error) {
	if hasDeleteResourcesFinalizer(st.bundle) {
		if !hasFinalizer(st.bundle, meta_v1.FinalizerDeleteDependents) {
			// If "foregroundDeletion" finalizer was not set, perform manual cascade deletion
			retrieable, err := st.deleteAllResources()
			if err != nil {
				return retrieable, err
			}
		}

		// If the "foregroundDeletion" finalizer is set, or the manual deletion
		// of resources has succeeded, remove the "deleteResources" finalizer
		st.newFinalizers = removeDeleteResourcesFinalizer(st.bundle.GetFinalizers())
	}
	return false, nil
}

// TODO: delete this method after the issue in kubectl is fixed:
func (st *bundleSyncTask) deleteAllResources() (retriableError bool, e error) {
	objs, err := st.store.ObjectsControlledBy(st.bundle.Namespace, st.bundle.UID)
	if err != nil {
		return false, err
	}
	existingObjs := make(map[objectRef]types.UID, len(objs))
	for _, obj := range objs {
		m := obj.(meta_v1.Object)
		if m.GetDeletionTimestamp() != nil {
			// Object is marked for deletion already
			continue
		}
		ref := objectRef{
			GroupVersionKind: obj.GetObjectKind().GroupVersionKind(),
			Name:             m.GetName(),
		}
		existingObjs[ref] = m.GetUID()
	}
	var firstErr error
	retriable := true
	policy := meta_v1.DeletePropagationForeground
	for ref, uid := range existingObjs {
		st.logger.Info("Deleting object", logz.Gvk(ref.GroupVersionKind), logz.ObjectName(ref.Name))
		resClient, err := st.smartClient.ForGVK(ref.GroupVersionKind, st.bundle.Namespace)
		if err != nil {
			if firstErr == nil {
				retriable = false
				firstErr = err
			} else {
				st.logger.Error("Failed to get client for object", logz.Gvk(ref.GroupVersionKind), zap.Error(err))
			}
			continue
		}

		err = resClient.Delete(ref.Name, &meta_v1.DeleteOptions{
			Preconditions: &meta_v1.Preconditions{
				UID: &uid,
			},
			PropagationPolicy: &policy,
		})
		if err != nil && !api_errors.IsNotFound(err) && !api_errors.IsConflict(err) {
			// not found means object has been deleted already
			// conflict means it has been deleted and re-created (UID does not match)
			if firstErr == nil {
				firstErr = err
			} else {
				st.logger.Warn("Failed to delete object", logz.Gvk(ref.GroupVersionKind), logz.ObjectName(ref.Name), zap.Error(err))
			}
			continue
		}
	}
	return retriable, firstErr
}

func (st *bundleSyncTask) deleteRemovedResources() (retriableError bool, e error) {
	objs, err := st.store.ObjectsControlledBy(st.bundle.Namespace, st.bundle.UID)
	if err != nil {
		return false, err
	}
	existingObjs := make(map[objectRef]types.UID, len(objs))
	for _, obj := range objs {
		m := obj.(meta_v1.Object)
		if m.GetDeletionTimestamp() != nil {
			// Object is marked for deletion already
			continue
		}
		ref := objectRef{
			GroupVersionKind: obj.GetObjectKind().GroupVersionKind(),
			Name:             m.GetName(),
		}
		existingObjs[ref] = m.GetUID()
	}
	for _, res := range st.bundle.Spec.Resources {
		var gvk schema.GroupVersionKind
		var name string
		if res.Spec.Object != nil {
			gvk = res.Spec.Object.GetObjectKind().GroupVersionKind()
			name = res.Spec.Object.(meta_v1.Object).GetName()
		} else if res.Spec.Plugin != nil {
			gvk = st.pluginContainers[res.Spec.Plugin.Name].Plugin.Describe().GVK
			name = res.Spec.Plugin.ObjectName
		} else {
			return false, errors.New(`neither "object" nor "plugin" field is specified`)
		}
		ref := objectRef{
			GroupVersionKind: gvk,
			Name:             name,
		}
		delete(existingObjs, ref)
	}
	var firstErr error
	retriable := true
	policy := meta_v1.DeletePropagationForeground
	for ref, uid := range existingObjs {
		st.logger.Info("Deleting object", logz.Gvk(ref.GroupVersionKind), logz.ObjectName(ref.Name))
		resClient, err := st.smartClient.ForGVK(ref.GroupVersionKind, st.bundle.Namespace)
		if err != nil {
			if firstErr == nil {
				retriable = false
				firstErr = err
			} else {
				st.logger.Error("Failed to get client for object", logz.Gvk(ref.GroupVersionKind), zap.Error(err))
			}
			continue
		}

		err = resClient.Delete(ref.Name, &meta_v1.DeleteOptions{
			Preconditions: &meta_v1.Preconditions{
				UID: &uid,
			},
			PropagationPolicy: &policy,
		})
		if err != nil && !api_errors.IsNotFound(err) && !api_errors.IsConflict(err) {
			// not found means object has been deleted already
			// conflict means it has been deleted and re-created (UID does not match)
			if firstErr == nil {
				firstErr = err
			} else {
				st.logger.Warn("Failed to delete object", logz.Gvk(ref.GroupVersionKind), logz.ObjectName(ref.Name), zap.Error(err))
			}
			continue
		}
	}
	return retriable, firstErr
}

func (st *bundleSyncTask) updateBundle() error {
	bundleUpdated, err := st.bundleClient.Bundles(st.bundle.Namespace).Update(st.bundle)
	if err != nil {
		if api_errors.IsConflict(err) {
			// Something updated the bundle concurrently.
			// It is possible that it was us in previous iteration but we haven't observed the
			// resulting update event for the bundle and this iteration was triggered by something
			// else e.g. resource update.
			// It is safe to ignore this conflict because we will reiterate because of the update event.
			return nil
		}
		return errors.Wrap(err, "failed to update bundle")
	}
	st.logger.Sugar().Debugf("Set bundle status to %s", &bundleUpdated.Status)
	st.logger.Sugar().Debugf("Set bundle finalizers to %#v", &bundleUpdated.Finalizers)
	return nil
}

func (st *bundleSyncTask) handleProcessResult(retriable bool, processErr error) (bool /*retriable*/, error) {
	if processErr != nil && api_errors.IsConflict(errors.Cause(processErr)) {
		return retriable, processErr
	}
	if processErr == context.Canceled || processErr == context.DeadlineExceeded {
		return false, processErr
	}

	// Construct resource conditions and check if there were any resource errors
	resourceStatuses := make([]smith_v1.ResourceStatus, 0, len(st.processedResources))
	var failedResources []smith_v1.ResourceName
	retriableResourceErr := true
	statusUpdated := false
	for _, res := range st.bundle.Spec.Resources { // Deterministic iteration order
		blockedCond := smith_v1.ResourceCondition{Type: smith_v1.ResourceBlocked, Status: smith_v1.ConditionFalse}
		inProgressCond := smith_v1.ResourceCondition{Type: smith_v1.ResourceInProgress, Status: smith_v1.ConditionFalse}
		readyCond := smith_v1.ResourceCondition{Type: smith_v1.ResourceReady, Status: smith_v1.ConditionFalse}
		errorCond := smith_v1.ResourceCondition{Type: smith_v1.ResourceError, Status: smith_v1.ConditionFalse}

		if resInfo, ok := st.processedResources[res.Name]; ok {
			// Resource was processed
			switch resStatus := resInfo.status.(type) {
			case resourceStatusDependenciesNotReady:
				blockedCond.Status = smith_v1.ConditionTrue
				blockedCond.Reason = smith_v1.ResourceReasonDependenciesNotReady
				blockedCond.Message = fmt.Sprintf("Not ready: %q", resStatus.dependencies)
			case resourceStatusInProgress:
				inProgressCond.Status = smith_v1.ConditionTrue
			case resourceStatusReady:
				readyCond.Status = smith_v1.ConditionTrue
			case resourceStatusError:
				errorCond.Status = smith_v1.ConditionTrue
				errorCond.Message = resStatus.err.Error()
				if resStatus.isRetriableError {
					errorCond.Reason = smith_v1.ResourceReasonRetriableError
					inProgressCond.Status = smith_v1.ConditionTrue
				} else {
					errorCond.Reason = smith_v1.ResourceReasonTerminalError
				}
				failedResources = append(failedResources, res.Name)
				retriableResourceErr = retriableResourceErr && resStatus.isRetriableError // Must not continue if at least one error is not retriable
			default:
				blockedCond.Status = smith_v1.ConditionUnknown
				inProgressCond.Status = smith_v1.ConditionUnknown
				readyCond.Status = smith_v1.ConditionUnknown
				errorCond.Status = smith_v1.ConditionTrue
				errorCond.Reason = smith_v1.ResourceReasonTerminalError
				errorCond.Message = fmt.Sprintf("internal error - unknown resource status type %T", resInfo.status)
				failedResources = append(failedResources, res.Name)
				retriableResourceErr = false
			}
		} else {
			// Resource was not processed
			blockedCond.Status = smith_v1.ConditionUnknown
			inProgressCond.Status = smith_v1.ConditionUnknown
			readyCond.Status = smith_v1.ConditionUnknown
			errorCond.Status = smith_v1.ConditionUnknown
		}
		statusUpdated = updateResourceCondition(st.bundle, res.Name, &blockedCond) || statusUpdated
		statusUpdated = updateResourceCondition(st.bundle, res.Name, &inProgressCond) || statusUpdated
		statusUpdated = updateResourceCondition(st.bundle, res.Name, &readyCond) || statusUpdated
		statusUpdated = updateResourceCondition(st.bundle, res.Name, &errorCond) || statusUpdated
		resourceStatuses = append(resourceStatuses, smith_v1.ResourceStatus{
			Name:       res.Name,
			Conditions: []smith_v1.ResourceCondition{blockedCond, inProgressCond, readyCond, errorCond},
		})
	}

	if processErr == nil && len(failedResources) > 0 {
		processErr = errors.Errorf("error processing resource(s): %q", failedResources)
		retriable = retriableResourceErr
	}

	// Bundle conditions
	inProgressCond := smith_v1.BundleCondition{Type: smith_v1.BundleInProgress, Status: smith_v1.ConditionFalse}
	readyCond := smith_v1.BundleCondition{Type: smith_v1.BundleReady, Status: smith_v1.ConditionFalse}
	errorCond := smith_v1.BundleCondition{Type: smith_v1.BundleError, Status: smith_v1.ConditionFalse}
	if processErr == nil {
		if st.isBundleReady() {
			readyCond.Status = smith_v1.ConditionTrue
		} else {
			inProgressCond.Status = smith_v1.ConditionTrue
		}
	} else {
		errorCond.Status = smith_v1.ConditionTrue
		errorCond.Message = processErr.Error()
		if retriable {
			errorCond.Reason = smith_v1.BundleReasonRetriableError
			inProgressCond.Status = smith_v1.ConditionTrue
		} else {
			errorCond.Reason = smith_v1.BundleReasonTerminalError
		}
	}

	statusUpdated = updateBundleCondition(st.bundle, &inProgressCond) || statusUpdated
	statusUpdated = updateBundleCondition(st.bundle, &readyCond) || statusUpdated
	statusUpdated = updateBundleCondition(st.bundle, &errorCond) || statusUpdated

	// Update the bundle status
	if statusUpdated {
		st.bundle.Status.ResourceStatuses = resourceStatuses
		st.bundle.Status.Conditions = []smith_v1.BundleCondition{inProgressCond, readyCond, errorCond}
	}

	// Update finalizers
	finalizersUpdated := false
	if st.newFinalizers != nil {
		finalizersUpdated = true
		st.bundle.Finalizers = st.newFinalizers
	}

	if statusUpdated || finalizersUpdated {
		ex := st.updateBundle()
		if processErr == nil {
			processErr = ex
			retriable = true
		}
	}

	return retriable, processErr
}

func (st *bundleSyncTask) isBundleReady() bool {
	for _, res := range st.bundle.Spec.Resources {
		res := st.processedResources[res.Name]
		if res == nil || !res.isReady() {
			return false
		}
	}
	return true
}

// updateBundleCondition updates passed condition by fetching information from an existing resource condition if present.
// Sets LastTransitionTime to now if the status has changed.
// Returns true if resource condition in the bundle does not match and needs to be updated.
func updateBundleCondition(b *smith_v1.Bundle, condition *smith_v1.BundleCondition) bool {
	now := meta_v1.Now()
	condition.LastTransitionTime = now

	// Try to find resource condition
	_, oldCondition := b.GetCondition(condition.Type)

	if oldCondition == nil {
		// New resource condition
		return true
	}

	// We are updating an existing condition, so we need to check if it has changed.
	if condition.Status == oldCondition.Status {
		condition.LastTransitionTime = oldCondition.LastTransitionTime
	}

	isEqual := condition.Status == oldCondition.Status &&
		condition.Reason == oldCondition.Reason &&
		condition.Message == oldCondition.Message &&
		condition.LastTransitionTime.Equal(&oldCondition.LastTransitionTime)

	if !isEqual {
		condition.LastUpdateTime = now
	}

	// Return true if one of the fields have changed.
	return !isEqual
}

// updateResourceCondition updates passed condition by fetching information from an existing resource condition if present.
// Sets LastTransitionTime to now if the status has changed.
// Returns true if resource condition in the bundle does not match and needs to be updated.
func updateResourceCondition(b *smith_v1.Bundle, resName smith_v1.ResourceName, condition *smith_v1.ResourceCondition) bool {
	now := meta_v1.Now()
	condition.LastTransitionTime = now
	// Try to find this resource status
	_, status := b.Status.GetResourceStatus(resName)

	if status == nil {
		// No status for this resource, hence it's a new resource condition
		return true
	}

	// Try to find resource condition
	_, oldCondition := status.GetCondition(condition.Type)

	if oldCondition == nil {
		// New resource condition
		return true
	}

	// We are updating an existing condition, so we need to check if it has changed.
	if condition.Status == oldCondition.Status {
		condition.LastTransitionTime = oldCondition.LastTransitionTime
	}

	isEqual := condition.Status == oldCondition.Status &&
		condition.Reason == oldCondition.Reason &&
		condition.Message == oldCondition.Message &&
		condition.LastTransitionTime.Equal(&oldCondition.LastTransitionTime)

	if !isEqual {
		condition.LastUpdateTime = now
	}

	// Return true if one of the fields have changed.
	return !isEqual
}

func sortBundle(bundle *smith_v1.Bundle) (*graph.Graph, []graph.V, error) {
	g := graph.NewGraph(len(bundle.Spec.Resources))

	for _, res := range bundle.Spec.Resources {
		g.AddVertex(graph.V(res.Name), nil)
	}

	for _, res := range bundle.Spec.Resources {
		for _, d := range res.DependsOn {
			if err := g.AddEdge(res.Name, d); err != nil {
				return nil, nil, err
			}
		}
	}

	sorted, err := g.TopologicalSort()
	if err != nil {
		return nil, nil, err
	}

	return g, sorted, nil
}
