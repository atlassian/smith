package controller

import (
	"context"
	"fmt"
	"log"

	"github.com/atlassian/smith"
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	smithClient_v1 "github.com/atlassian/smith/pkg/client/clientset_generated/clientset/typed/smith/v1"
	"github.com/atlassian/smith/pkg/plugin"
	"github.com/atlassian/smith/pkg/util/graph"

	"github.com/pkg/errors"
	api_errors "k8s.io/apimachinery/pkg/api/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

type bundleSyncTask struct {
	bundleClient       smithClient_v1.BundlesGetter
	smartClient        smith.SmartClient
	rc                 ReadyChecker
	store              Store
	specCheck          SpecCheck
	bundle             *smith_v1.Bundle
	processedResources map[smith_v1.ResourceName]*resourceInfo
	plugins            map[smith_v1.PluginName]plugin.Plugin
	scheme             *runtime.Scheme
}

// Parse bundle, build resource graph, traverse graph, assert each resource exists.
// For each resource ensure its dependencies (if any) are in READY state before creating it.
// If at least one dependency is not READY - skip the resource. Rebuild will/should be called once the dependency
// updates it's state (noticed via watching).

// READY state might mean something different for each resource type. For a Custom Resource it may mean
// that a field "State" in the Status of the resource is set to "Ready". It is customizable via
// annotations with some defaults.
func (st *bundleSyncTask) process() (retriableError bool, e error) {
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
		res := resourceMap[resName.(smith_v1.ResourceName)]
		rst := resourceSyncTask{
			smartClient:        st.smartClient,
			rc:                 st.rc,
			store:              st.store,
			specCheck:          st.specCheck,
			bundle:             st.bundle,
			processedResources: st.processedResources,
			plugins:            st.plugins,
			scheme:             st.scheme,
		}
		resInfo := rst.processResource(&res)
		if retriable, err := resInfo.fetchError(); err != nil && api_errors.IsConflict(errors.Cause(err)) {
			// Short circuit on conflict
			return retriable, err
		}
		log.Printf("[WORKER][%s/%s] Resource %q, ready: %t", st.bundle.Namespace, st.bundle.Name, resName, resInfo.isReady())
		st.processedResources[resName.(smith_v1.ResourceName)] = &resInfo
	}
	// Delete objects which were removed from the bundle
	retriable, err := st.deleteRemovedResources()
	if err != nil {
		return retriable, err
	}

	return false, nil
}

func (st *bundleSyncTask) deleteRemovedResources() (retriableError bool, e error) {
	objs, err := st.store.GetObjectsForBundle(st.bundle.Namespace, st.bundle.Name)
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
		if !meta_v1.IsControlledBy(m, st.bundle) {
			// Object is not owned by that bundle
			log.Printf("[WORKER][%s/%s] Object %v %q is not owned by the bundle with UID=%q. Owner references: %v",
				st.bundle.Namespace, st.bundle.Name, obj.GetObjectKind().GroupVersionKind(), m.GetName(), st.bundle.GetUID(), m.GetOwnerReferences())
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
			gvk = st.plugins[res.Spec.Plugin.Name].Describe().GVK
			name = res.Spec.Plugin.ObjectName
		} else {
			panic(errors.New(`neither "object" nor "plugin" field is specified`))
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
		log.Printf("[WORKER][%s/%s] Deleting object %v %q", st.bundle.Namespace, st.bundle.Name, ref.GroupVersionKind, ref.Name)
		resClient, err := st.smartClient.ForGVK(ref.GroupVersionKind, st.bundle.Namespace)
		if err != nil {
			if firstErr == nil {
				retriable = false
				firstErr = err
			} else {
				log.Printf("[WORKER][%s/%s] Failed to get client for object %s: %v", st.bundle.Namespace, st.bundle.Name, ref.GroupVersionKind, err)
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
				log.Printf("[WORKER][%s/%s] Failed to delete object %v %q: %v", st.bundle.Namespace, st.bundle.Name, ref.GroupVersionKind, ref.Name, err)
			}
			continue
		}
	}
	return retriable, firstErr
}

func (st *bundleSyncTask) setBundleStatus() error {
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
		return fmt.Errorf("failed to set bundle status to %s: %v", st.bundle.Status.ShortString(), err)
	}
	log.Printf("[WORKER][%s/%s] Set bundle status to %s", st.bundle.Namespace, st.bundle.Name, bundleUpdated.Status.ShortString())
	return nil
}

func (st *bundleSyncTask) handleProcessResult(retriable bool, processErr error) (bool /*retriable*/, error) {
	if processErr != nil && api_errors.IsConflict(errors.Cause(processErr)) {
		return retriable, processErr
	}
	if processErr == context.Canceled || processErr == context.DeadlineExceeded {
		return false, processErr
	}
	inProgressCond := smith_v1.BundleCondition{Type: smith_v1.BundleInProgress, Status: smith_v1.ConditionFalse}
	readyCond := smith_v1.BundleCondition{Type: smith_v1.BundleReady, Status: smith_v1.ConditionFalse}
	errorCond := smith_v1.BundleCondition{Type: smith_v1.BundleError, Status: smith_v1.ConditionFalse}
	if processErr == nil {
		// Check for errors in resources
		var failedResources []smith_v1.ResourceName
		for _, res := range st.bundle.Spec.Resources { // Deterministic iteration order
			resInfo := st.processedResources[res.Name]
			if isRetriableErr, err := resInfo.fetchError(); err != nil {
				failedResources = append(failedResources, res.Name)
				retriable = retriable || isRetriableErr // If at least one error is retriable...
			}
		}
		if len(failedResources) > 0 {
			processErr = errors.Errorf("error processing resource(s): %q", failedResources)
		}
	}
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

	inProgressUpdated := st.bundle.UpdateCondition(&inProgressCond)
	readyUpdated := st.bundle.UpdateCondition(&readyCond)
	errorUpdated := st.bundle.UpdateCondition(&errorCond)

	// Updating the bundle state
	if inProgressUpdated || readyUpdated || errorUpdated {
		ex := st.setBundleStatus()
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
