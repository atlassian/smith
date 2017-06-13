package processor

import (
	"context"
	"fmt"
	"log"
	"sync/atomic"
	"time"

	"github.com/atlassian/smith"
	"github.com/atlassian/smith/pkg/processor/graph"
	"github.com/atlassian/smith/pkg/resources"

	"github.com/cenk/backoff"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	unstructured_conversion "k8s.io/apimachinery/pkg/conversion/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

var (
	converter = unstructured_conversion.NewConverter(false)
)

type workerConfig struct {
	bundleClient *rest.RESTClient
	sc           smith.SmartClient
	rc           ReadyChecker
	deepCopy     smith.DeepCopy
	store        Store
}

type worker struct {
	bundleRef
	workerConfig
	bo             backoff.BackOff
	workRequests   chan<- workRequest
	notifyRequests chan<- notifyRequest

	// --- Items below are only modified from main processor goroutine ---
	pendingBundle *smith.Bundle   // next work item storage
	notify        chan<- struct{} // channel to close to notify sleeping worker about work available
	// ---

	needsRebuild int32 // must be accessed via atomics only
}

func (wrk *worker) rebuildLoop(ctx context.Context) {
	var timer *time.Timer
	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()
	work := make(chan *smith.Bundle)
	workReq := workRequest{
		bundleRef: wrk.bundleRef,
		work:      work,
	}
	var notifyChan <-chan struct{}
	var sleepChan <-chan time.Time
	requestWork := true
	for {
		if requestWork {
			select {
			case <-ctx.Done():
				return
			case wrk.workRequests <- workReq:
				// New work has been requested
			}
		} else {
			requestWork = true
		}
		select {
		case <-ctx.Done():
			return
		case <-notifyChan:
			// Woke up because work is available
			sleepChan = nil
			notifyChan = nil
			timer.Stop()
			timer = nil
		case <-sleepChan:
			// Woke up after backoff sleep
			sleepChan = nil
			notifyChan = nil
			timer = nil
		case bundle, ok := <-work:
			if !ok {
				// No work available, exit
				return
			}
			// Work available
			if bundle.DeletionTimestamp != nil {
				continue
			}
			isReady, retriable, err := wrk.rebuild(bundle)
			if wrk.handleRebuildResult(bundle, isReady, retriable, err) {
				// There was a retriable error
				next := wrk.bo.NextBackOff()
				if next == backoff.Stop {
					wrk.bo.Reset() // reset the backoff to restart it
					next = wrk.bo.NextBackOff()
				}
				timer = time.NewTimer(next)
				sleepChan = timer.C
				nc := make(chan struct{})
				notifyChan = nc
				nrq := notifyRequest{
					bundleRef: wrk.bundleRef,
					bundle:    bundle,
					notify:    nc,
				}
				select {
				case <-ctx.Done():
					return
				case wrk.notifyRequests <- nrq:
					// Asked processor to notify us if/when new work is available
				}
				requestWork = false
			}
		}
	}
}

// Parse bundle, build resource graph, traverse graph, assert each resource exists.
// For each resource ensure its dependencies (if any) are in READY state before creating it.
// If at least one dependency is not READY, exit loop. Rebuild will/should be called once the dependency
// updates it's state (noticed via watching).

// READY state might mean something different for each resource type. For ThirdPartyResources it may mean
// that a field "State" in the Status of the resource is set to "Ready". It may be customizable via
// annotations with some defaults.
func (wrk *worker) rebuild(bundle *smith.Bundle) (isReady, retriableError bool, e error) {
	log.Printf("[WORKER][%s/%s] Rebuilding bundle", wrk.namespace, wrk.bundleName)

	// Build resource map by name
	resourceMap := make(map[smith.ResourceName]smith.Resource, len(bundle.Spec.Resources))
	for _, res := range bundle.Spec.Resources {
		if _, exist := resourceMap[res.Name]; exist {
			return false, false, fmt.Errorf("bundle contains two resources with the same name %q", res.Name)
		}
		resourceMap[res.Name] = res
	}

	// Build the graph and topologically sort it
	graphData, sortErr := graph.TopologicalSort(bundle)
	if sortErr != nil {
		return false, false, sortErr
	}

	readyResources := make(map[smith.ResourceName]*unstructured.Unstructured, len(bundle.Spec.Resources))
	allReady := true

	// Visit vertices in sorted order
nextVertex:
	for _, v := range graphData.SortedVertices {
		if wrk.checkNeedsRebuild() {
			return false, false, nil
		}

		// Check if all resource dependencies are ready (so we can start processing this one)
		for _, dependency := range graphData.Graph.Vertices[v].Edges() {
			if _, ok := readyResources[dependency]; !ok {
				allReady = false
				log.Printf("[WORKER][%s/%s] Dependency %q is required by resource %q but it's not ready", wrk.namespace, wrk.bundleName, dependency, v)
				continue nextVertex // Move to the next resource
			}
		}
		// Process the resource
		log.Printf("[WORKER][%s/%s] Checking resource %q", wrk.namespace, wrk.bundleName, v)
		res := resourceMap[v]
		readyResource, retriable, err := wrk.checkResource(bundle, &res, readyResources)
		if err != nil {
			return false, retriable, err
		}
		log.Printf("[WORKER][%s/%s] Resource %q, ready: %t", wrk.namespace, wrk.bundleName, v, readyResource != nil)
		if readyResource != nil {
			readyResources[v] = readyResource
		} else {
			allReady = false
		}
	}
	// Delete objects which were removed from the bundle
	retriable, err := wrk.deleteRemovedResources(bundle)
	if err != nil {
		return false, retriable, err
	}

	return allReady, false, nil
}

func (wrk *worker) checkResource(bundle *smith.Bundle, res *smith.Resource, readyResources map[smith.ResourceName]*unstructured.Unstructured) (readyResource *unstructured.Unstructured, retriableError bool, e error) {
	// 1. Eval spec
	spec, err := wrk.evalSpec(bundle, res, readyResources)
	if err != nil {
		return nil, false, err
	}

	// 2. Create or update resource
	resUpdated, retriable, err := wrk.createOrUpdate(bundle, spec)
	if err != nil || resUpdated == nil {
		return nil, retriable, err
	}

	// 3. Check if resource is ready
	ready, retriable, err := wrk.rc.IsReady(resUpdated)
	if err != nil || !ready {
		return nil, retriable, err
	}
	return resUpdated, false, nil
}

// evalSpec evaluates the resource specification and returns the result.
func (wrk *worker) evalSpec(bundle *smith.Bundle, res *smith.Resource, readyResources map[smith.ResourceName]*unstructured.Unstructured) (*unstructured.Unstructured, error) {
	// 0. Convert to Unstructured
	spec, err := res.ToUnstructured(wrk.deepCopy)
	if err != nil {
		return nil, err
	}

	// 1. Process references
	sp := NewSpec(res.Name, readyResources, res.DependsOn)
	if err := sp.ProcessObject(spec.Object); err != nil {
		return nil, err
	}

	// 2. Update label to point at the parent bundle
	spec.SetLabels(mergeLabels(
		bundle.Labels,
		spec.GetLabels(),
		map[string]string{smith.BundleNameLabel: wrk.bundleName}))

	// 3. Update OwnerReferences
	trueRef := true
	refs := spec.GetOwnerReferences()
	for i, ref := range refs {
		if ref.Controller != nil && *ref.Controller {
			return nil, fmt.Errorf("cannot create resource %q with controller owner reference %v", res.Name, ref)
		}
		refs[i].BlockOwnerDeletion = &trueRef
	}
	// Hardcode APIVersion/Kind because of https://github.com/kubernetes/client-go/issues/60
	refs = append(refs, meta_v1.OwnerReference{
		APIVersion:         smith.BundleResourceGroupVersion,
		Kind:               smith.BundleResourceKind,
		Name:               bundle.Name,
		UID:                bundle.UID,
		Controller:         &trueRef,
		BlockOwnerDeletion: &trueRef,
	})
	for _, dep := range res.DependsOn {
		obj := readyResources[dep] // this is ok because we've checked earlier that readyResources contains all dependencies
		refs = append(refs, meta_v1.OwnerReference{
			APIVersion:         obj.GetAPIVersion(),
			Kind:               obj.GetKind(),
			Name:               obj.GetName(),
			UID:                obj.GetUID(),
			BlockOwnerDeletion: &trueRef,
		})
	}
	spec.SetOwnerReferences(refs)

	return spec, nil
}

// createOrUpdate creates or updates a resources.
// May return nil resource without any errors if an update/create conflict happened.
func (wrk *worker) createOrUpdate(bundle *smith.Bundle, spec *unstructured.Unstructured) (resUpdated *unstructured.Unstructured, retriableError bool, e error) {
	// Prepare client
	resClient, err := wrk.sc.ForGVK(spec.GroupVersionKind(), wrk.namespace)
	if err != nil {
		return nil, false, err
	}
	gvk := spec.GetObjectKind().GroupVersionKind()

	// Try to get the resource. We do read first to avoid generating unnecessary events.
	obj, exists, err := wrk.store.Get(gvk, wrk.namespace, spec.GetName())
	if err != nil {
		// Unexpected error
		return nil, false, err
	}
	if exists {
		return wrk.updateResource(bundle, resClient, spec, obj)
	}
	return wrk.createResource(resClient, spec)
}

func (wrk *worker) createResource(resClient *dynamic.ResourceClient, spec *unstructured.Unstructured) (resUpdated *unstructured.Unstructured, retriableError bool, e error) {
	log.Printf("[WORKER][%s/%s] Object %q not found, creating", wrk.namespace, wrk.bundleName, spec.GetName())
	response, err := resClient.Create(spec)
	if err == nil {
		log.Printf("[WORKER][%s/%s] Object %q created", wrk.namespace, wrk.bundleName, spec.GetName())
		return response, false, nil
	}
	if errors.IsAlreadyExists(err) {
		log.Printf("[WORKER][%s/%s] Object %q found, but not in Store yet, restarting loop", wrk.namespace, wrk.bundleName, spec.GetName())
		// We let the next rebuild() iteration, triggered by someone else creating the resource, to finish the work.
		return nil, false, nil
	}
	// Unexpected error, will retry
	return nil, true, err
}

func (wrk *worker) updateResource(bundle *smith.Bundle, resClient *dynamic.ResourceClient, spec *unstructured.Unstructured, obj runtime.Object) (resUpdated *unstructured.Unstructured, retriableError bool, e error) {
	existingObj, ok := obj.(*unstructured.Unstructured)
	if !ok {
		existingObj = &unstructured.Unstructured{
			Object: make(map[string]interface{}),
		}
		if err := converter.ToUnstructured(obj, &existingObj.Object); err != nil {
			// Unexpected error
			return nil, false, err
		}
	}

	// Check that the object is not marked for deletion
	if existingObj.GetDeletionTimestamp() != nil {
		return nil, false, fmt.Errorf("object %v %q is marked for deletion", existingObj.GroupVersionKind(), existingObj.GetName())
	}

	// Check that this bundle owns the object
	if !isOwner(existingObj, bundle) {
		return nil, false, fmt.Errorf("object %v %q is not owned by the Bundle", existingObj.GroupVersionKind(), existingObj.GetName())
	}

	// Compare spec and existing resource
	updated, err := updateResource(wrk.deepCopy, spec, existingObj)
	if err != nil {
		// Unexpected error
		return nil, false, err
	}
	if updated == nil {
		log.Printf("[WORKER][%s/%s] Object %q has correct spec", wrk.namespace, wrk.bundleName, spec.GetName())
		return existingObj, false, nil
	}

	// Update if different
	existingObj, err = resClient.Update(updated)
	if err != nil {
		if errors.IsConflict(err) {
			log.Printf("[WORKER][%s/%s] Object %q update resulted in conflict, restarting loop", wrk.namespace, wrk.bundleName, spec.GetName())
			// We let the next rebuild() iteration, triggered by someone else updating the resource, to finish the work.
			return nil, false, nil
		}
		// Unexpected error, will retry
		return nil, true, err
	}
	log.Printf("[WORKER][%s/%s] Object %q updated", wrk.namespace, wrk.bundleName, spec.GetName())
	return existingObj, false, nil
}

func (wrk *worker) deleteRemovedResources(bundle *smith.Bundle) (retriableError bool, e error) {
	objs, err := wrk.store.GetObjectsForBundle(wrk.namespace, wrk.bundleName)
	if err != nil {
		return false, err
	}
	existingObjs := make(map[objectRef]types.UID, len(objs))
	for _, obj := range objs {
		m, err := meta.Accessor(obj)
		if err != nil {
			return false, fmt.Errorf("failed to get meta of object: %v", err)
		}
		if m.GetDeletionTimestamp() != nil {
			// Object is marked for deletion already
			continue
		}
		if !isOwner(m, bundle) {
			// Object is not owned by that bundle
			log.Printf("[WORKER][%s/%s] Object %v %q is not owned by the bundle with UID=%q. Owner references: %v",
				wrk.namespace, wrk.bundleName, obj.GetObjectKind().GroupVersionKind(), m.GetName(), bundle.GetUID(), m.GetOwnerReferences())
			continue
		}
		ref := objectRef{
			GroupVersionKind: obj.GetObjectKind().GroupVersionKind(),
			Name:             m.GetName(),
		}
		existingObjs[ref] = m.GetUID()
	}
	for _, res := range bundle.Spec.Resources {
		m, err := meta.Accessor(res.Spec)
		if err != nil {
			return false, fmt.Errorf("failed to get meta of object: %v", err)
		}
		ref := objectRef{
			GroupVersionKind: res.Spec.GetObjectKind().GroupVersionKind(),
			Name:             m.GetName(),
		}
		delete(existingObjs, ref)
	}
	var firstErr error
	retriable := true
	policy := meta_v1.DeletePropagationForeground
	for ref, uid := range existingObjs {
		log.Printf("[WORKER][%s/%s] Deleting object %v %q", wrk.namespace, wrk.bundleName, ref.GroupVersionKind, ref.Name)
		resClient, err := wrk.sc.ForGVK(ref.GroupVersionKind, wrk.namespace)
		if err != nil {
			if firstErr == nil {
				retriable = false
				firstErr = err
			} else {
				log.Printf("[WORKER][%s/%s] Failed to get client for object %s: %v", wrk.namespace, wrk.bundleName, ref.GroupVersionKind, err)
			}
			continue
		}

		err = resClient.Delete(ref.Name, &meta_v1.DeleteOptions{
			Preconditions: &meta_v1.Preconditions{
				UID: &uid,
			},
			PropagationPolicy: &policy,
		})
		if err != nil && !kerrors.IsNotFound(err) && !kerrors.IsConflict(err) {
			// not found means object has been deleted already
			// conflict means it has been deleted and re-created (UID does not match)
			if firstErr == nil {
				firstErr = err
			} else {
				log.Printf("[WORKER][%s/%s] Failed to delete object %v %q: %v", wrk.namespace, wrk.bundleName, ref.GroupVersionKind, ref.Name, err)
			}
			continue
		}
	}
	return retriable, firstErr
}

func (wrk *worker) setBundleStatus(bundle *smith.Bundle) error {
	log.Printf("[WORKER][%s/%s] Setting bundle status to %v", wrk.namespace, wrk.bundleName, bundle.Status)
	err := wrk.bundleClient.Put().
		Namespace(wrk.namespace).
		Resource(smith.BundleResourcePath).
		Name(wrk.bundleName).
		Body(bundle).
		Do().
		Into(bundle)
	if err != nil {
		if kerrors.IsConflict(err) {
			// Something updated the bundle concurrently.
			// It is possible that it was us in previous iteration but we haven't observed the
			// resulting update event for the bundle and this iteration was triggered by something
			// else e.g. resource update.
			// It is safe to ignore this conflict because we will reiterate because of the update event.
			return nil
		}
		return fmt.Errorf("failed to set bundle %s/%s status to %v: %v", wrk.namespace, wrk.bundleName, bundle.Status, err)
	}
	return nil
}

func (wrk *worker) handleRebuildResult(bundle *smith.Bundle, isReady, retriableError bool, err error) (shouldBackoff bool) {
	if err == context.Canceled || err == context.DeadlineExceeded {
		return false
	}
	var inProgressCond, readyCond, errorCond smith.BundleCondition
	if err == nil {
		errorCond = smith.BundleCondition{
			Type:   smith.BundleError,
			Status: smith.ConditionFalse,
		}
		if isReady {
			inProgressCond = smith.BundleCondition{
				Type:   smith.BundleInProgress,
				Status: smith.ConditionFalse,
			}
			readyCond = smith.BundleCondition{
				Type:   smith.BundleReady,
				Status: smith.ConditionTrue,
			}
		} else {
			inProgressCond = smith.BundleCondition{
				Type:   smith.BundleInProgress,
				Status: smith.ConditionTrue,
			}
			readyCond = smith.BundleCondition{
				Type:   smith.BundleReady,
				Status: smith.ConditionFalse,
			}
		}
	} else {
		readyCond = smith.BundleCondition{
			Type:   smith.BundleReady,
			Status: smith.ConditionFalse,
		}
		if retriableError {
			errorCond = smith.BundleCondition{
				Type:    smith.BundleError,
				Status:  smith.ConditionTrue,
				Reason:  "RetriableError",
				Message: err.Error(),
			}
			inProgressCond = smith.BundleCondition{
				Type:   smith.BundleInProgress,
				Status: smith.ConditionTrue,
			}
		} else {
			errorCond = smith.BundleCondition{
				Type:    smith.BundleError,
				Status:  smith.ConditionTrue,
				Reason:  "TerminalError",
				Message: err.Error(),
			}
			inProgressCond = smith.BundleCondition{
				Type:   smith.BundleInProgress,
				Status: smith.ConditionFalse,
			}
		}
	}

	inProgressUpdated := bundle.UpdateCondition(&inProgressCond)
	readyUpdated := bundle.UpdateCondition(&readyCond)
	errorUpdated := bundle.UpdateCondition(&errorCond)

	// Updating the bundle state
	if inProgressUpdated || readyUpdated || errorUpdated {
		ex := wrk.setBundleStatus(bundle)
		if err == nil {
			err = ex
		}
	}
	if err == nil {
		wrk.bo.Reset() // reset the backoff on successful rebuild
		return false
	}

	log.Printf("[WORKER][%s/%s] Failed to rebuild bundle (retriable: %t): %v", wrk.namespace, wrk.bundleName, retriableError, err)
	// If error is not retriable then we just tell the external loop to re-iterate without backoff
	// and terminates naturally unless new work is available.
	return retriableError
}

func (wrk *worker) setNeedsRebuild() {
	atomic.StoreInt32(&wrk.needsRebuild, 1)
}

func (wrk *worker) resetNeedsRebuild() {
	atomic.StoreInt32(&wrk.needsRebuild, 0)
}

// checkNeedsRebuild can be called inside of the rebuild loop to check if the bundle needs to be rebuilt from the start.
func (wrk *worker) checkNeedsRebuild() bool {
	return atomic.LoadInt32(&wrk.needsRebuild) != 0
}

// updateResource checks if actual resource satisfies the desired spec.
// Returns non-nil object with updates applied or nil if actual matches desired.
func updateResource(deepCopy smith.DeepCopy, spec, actual *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	// TODO Handle deleted or to be deleted object. We need to wait for the object to be gone.

	upd, err := deepCopy(actual)
	if err != nil {
		return nil, err
	}
	updated := upd.(*unstructured.Unstructured)
	delete(updated.Object, "status")

	actClone, err := deepCopy(updated)
	if err != nil {
		return nil, err
	}
	actualClone := actClone.(*unstructured.Unstructured)

	// This is to ensure those fields actually exist in underlying map whether they are nil or empty slices/map
	actualClone.SetKind(spec.GetKind())             // Objects from type-specific informers don't have kind/api version
	actualClone.SetAPIVersion(spec.GetAPIVersion()) // Objects from type-specific informers don't have kind/api version
	actualClone.SetName(actualClone.GetName())
	actualClone.SetLabels(actualClone.GetLabels())
	actualClone.SetAnnotations(actualClone.GetAnnotations())
	actualClone.SetOwnerReferences(actualClone.GetOwnerReferences())
	actualClone.SetFinalizers(actualClone.GetFinalizers())

	// Remove status to make sure ready checker will only detect readiness after resource controller has seen
	// the object.
	// Will be possible to implement it in a cleaner way once "status" is a separate sub-resource.
	// See https://github.com/kubernetes/kubernetes/issues/38113
	// Also ideally we don't want to clear the status at all but have a way to tell if controller has
	// observed the update yet. Like Generation/ObservedGeneration for built-in controllers.
	delete(spec.Object, "status")

	// 1. TypeMeta
	updated.SetKind(spec.GetKind())
	updated.SetAPIVersion(spec.GetAPIVersion())

	// 2. Some stuff from ObjectMeta
	// TODO Ignores added annotations/labels. Should be configurable per-object and/or per-object kind?
	updated.SetName(spec.GetName())
	updated.SetLabels(spec.GetLabels())
	updated.SetAnnotations(spec.GetAnnotations())
	updated.SetOwnerReferences(spec.GetOwnerReferences()) // TODO Is this ok? Check that there is only one controller and it is THIS bundle
	updated.SetFinalizers(spec.GetFinalizers())           // TODO Is this ok?

	// 3. Everything else
	for field, value := range spec.Object {
		switch field {
		case "kind", "apiVersion", "metadata":
			continue
		}
		valueClone, err := deepCopy(value)
		if err != nil {
			return nil, err
		}
		updated.Object[field] = valueClone
	}

	if !equality.Semantic.DeepEqual(updated, actualClone) {
		return updated, nil
	}
	return nil, nil
}

func mergeLabels(labels ...map[string]string) map[string]string {
	result := make(map[string]string)
	for _, m := range labels {
		for k, v := range m {
			result[k] = v
		}
	}
	return result
}

func isOwner(obj meta_v1.Object, bundle *smith.Bundle) bool {
	ref := resources.GetControllerOf(obj)
	return ref != nil &&
		ref.APIVersion == smith.BundleResourceGroupVersion &&
		ref.Kind == smith.BundleResourceKind &&
		ref.Name == bundle.Name &&
		ref.UID == bundle.UID
}
