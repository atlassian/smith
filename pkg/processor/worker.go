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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	unstructured_conversion "k8s.io/apimachinery/pkg/conversion/unstructured"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

var (
	converter = unstructured_conversion.NewConverter(false)
)

type workerConfig struct {
	bundleClient *rest.RESTClient
	scDynamic    dynamic.ClientPool
	clients      dynamic.ClientPool
	rc           ReadyChecker
	deepCopy     smith.DeepCopy
	store        smith.ByNameStore
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

func (wrk *worker) rebuildLoop(ctx context.Context, done func()) {
	defer done()
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
			return false, false, fmt.Errorf("bundle %s/%s contains two resources with the same name %q", wrk.namespace, wrk.bundleName, res.Name)
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
				log.Printf("[WORKER][%s/%s] Dependencies are not ready for resource %q", wrk.namespace, wrk.bundleName, v)
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
	return allReady, false, nil
}

func (wrk *worker) checkResource(bundle *smith.Bundle, res *smith.Resource, readyResources map[smith.ResourceName]*unstructured.Unstructured) (readyResource *unstructured.Unstructured, retriableError bool, e error) {
	// 0. Clone before mutating
	resClone, err := wrk.deepCopy(res)
	if err != nil {
		return nil, false, err
	}
	res = resClone.(*smith.Resource)

	// 1. Eval spec
	if err = wrk.evalSpec(bundle, res, readyResources); err != nil {
		return nil, false, err
	}

	// 2. Create or update resource
	resUpdated, retriable, err := wrk.createOrUpdate(res)
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

// Mutates spec in place.
func (wrk *worker) evalSpec(bundle *smith.Bundle, res *smith.Resource, readyResources map[smith.ResourceName]*unstructured.Unstructured) error {
	// 1. Process references
	sp := NewSpec(res.Name, readyResources, res.DependsOn)
	if err := sp.ProcessObject(res.Spec.Object); err != nil {
		return err
	}

	// 2. Update label to point at the parent bundle
	res.Spec.SetLabels(mergeLabels(
		bundle.Labels,
		res.Spec.GetLabels(),
		map[string]string{smith.BundleNameLabel: wrk.bundleName}))

	// 3. Update OwnerReferences
	// Hardcode APIVersion/Kind because of https://github.com/kubernetes/client-go/issues/60
	// TODO uncomment when https://github.com/kubernetes/kubernetes/issues/39816 is fixed
	//res.Spec.SetOwnerReferences(append(res.Spec.GetOwnerReferences(), metav1.OwnerReference{
	//	APIVersion: smith.BundleResourceVersion,
	//	Kind:       smith.BundleResourceKind,
	//	Name:       bundle.Name,
	//	UID:        bundle.UID,
	//}))

	return nil
}

// createOrUpdate creates or updates a resources.
// May return nil resource without any errors if an update/create conflict happened.
func (wrk *worker) createOrUpdate(res *smith.Resource) (resUpdated *unstructured.Unstructured, retriableError bool, e error) {
	// 1. Prepare client
	resClient, err := resources.ClientForResource(&res.Spec, wrk.clients, wrk.scDynamic, wrk.namespace)
	if err != nil {
		return nil, false, err
	}
	gvk := res.Spec.GetObjectKind().GroupVersionKind()

	// 2. Try to get the resource. We do read first to avoid generating unnecessary events.
	obj, exists, err := wrk.store.Get(gvk, wrk.namespace, res.Spec.GetName())
	if err != nil {
		// Unexpected error
		return nil, true, err
	}
	var response *unstructured.Unstructured
	if exists {
		var ok bool
		response, ok = obj.(*unstructured.Unstructured)
		if !ok {
			response = &unstructured.Unstructured{
				Object: make(map[string]interface{}),
			}
			if err = converter.ToUnstructured(obj, &response.Object); err != nil {
				// Unexpected error
				return nil, false, err
			}
		}
	} else {
		log.Printf("[WORKER][%s/%s] Resource %q not found, creating", wrk.namespace, wrk.bundleName, res.Name)
		// 3. Create if does not exist
		response, err = resClient.Create(&res.Spec)
		if err == nil {
			log.Printf("[WORKER][%s/%s] Resource %q created", wrk.namespace, wrk.bundleName, res.Name)
			return response, false, nil
		}
		if errors.IsAlreadyExists(err) {
			log.Printf("[WORKER][%s/%s] Resource %q found, but not in Store yet, restarting loop", wrk.namespace, wrk.bundleName, res.Name)
			// We let the next rebuild() iteration, triggered by someone else creating the resource, to finish the work.
			return nil, false, nil
		}
		// Unexpected error, will retry
		return nil, true, err
	}

	// 4. Compare spec and existing resource
	updated, err := updateResource(wrk.deepCopy, res, response)
	if err != nil {
		// Unexpected error
		return nil, false, err
	}
	if updated == nil {
		log.Printf("[WORKER][%s/%s] Resource %q has correct spec", wrk.namespace, wrk.bundleName, res.Name)
		return response, false, nil
	}

	// 5. Update if different
	response, err = resClient.Update(updated)
	if err != nil {
		if errors.IsConflict(err) {
			log.Printf("[WORKER][%s/%s] Resource %q update resulted in conflict, restarting loop", wrk.namespace, wrk.bundleName, res.Name)
			// We let the next rebuild() iteration, triggered by someone else updating the resource, to finish the work.
			return nil, false, nil
		}
		// Unexpected error, will retry
		return nil, true, err
	}
	log.Printf("[WORKER][%s/%s] Resource %q updated", wrk.namespace, wrk.bundleName, res.Name)
	return response, false, nil
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
func updateResource(deepCopy smith.DeepCopy, desired *smith.Resource, actual *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	spec := desired.Spec

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
	updated.SetOwnerReferences(spec.GetOwnerReferences()) // TODO Is this ok?
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
