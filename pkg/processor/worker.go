package processor

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/atlassian/smith"
	"github.com/atlassian/smith/pkg/processor/graph"
	"github.com/atlassian/smith/pkg/resources"

	"github.com/cenk/backoff"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	unstructured_conversion "k8s.io/apimachinery/pkg/conversion/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	converter = unstructured_conversion.NewConverter(false)
)

type worker struct {
	bp *BundleProcessor
	bo backoff.BackOff
	workerRef

	// The fields below are protected by tp.lock

	// It is ok to mutate the bundle itself, but pointer should only be updated while locked.
	bundle *smith.Bundle
}

func (wrk *worker) rebuildLoop() {
	defer wrk.bp.wg.Done()
	defer wrk.cleanupState()

	for {
		bundle := wrk.checkRebuild()
		if bundle == nil {
			return
		}
		err := wrk.rebuild(bundle)
		if err == nil {
			wrk.bo.Reset() // reset the backoff on successful rebuild
			// Make sure bundle does not need to be rebuilt before exiting goroutine by doing one more iteration
		} else if !wrk.handleError(bundle, err) {
			return
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
func (wrk *worker) rebuild(bundle *smith.Bundle) error {
	if bundle.Status.State == smith.TERMINAL_ERROR {
		// Sad, but true.
		return nil
	}
	log.Printf("[WORKER] Rebuilding the bundle %s/%s", wrk.namespace, wrk.bundleName)

	// Build resource map by name
	resourceMap := make(map[smith.ResourceName]smith.Resource, len(bundle.Spec.Resources))
	for _, res := range bundle.Spec.Resources {
		resourceMap[res.Name] = res
	}

	// Build the graph and topologically sort it
	graphData, sortErr := graph.TopologicalSort(bundle)
	if sortErr != nil {
		return sortErr
	}

	readyResources := make(map[smith.ResourceName]*unstructured.Unstructured, len(bundle.Spec.Resources))
	allReady := true

	// Visit vertices in sorted order
nextVertex:
	for _, v := range graphData.SortedVertices {
		if wrk.checkNeedsRebuild() {
			return nil
		}

		// Check if all resource dependencies are ready (so we can start processing this one)
		for _, dependency := range graphData.Graph.Vertices[v].Edges() {
			if _, ok := readyResources[dependency]; !ok {
				allReady = false
				log.Printf("[WORKER] bundle %s/%s: dependencies are not ready for resource %q", wrk.namespace, wrk.bundleName, v)
				continue nextVertex // Move to the next resource
			}
		}
		// Process the resource
		log.Printf("[WORKER] bundle %s/%s: checking resource %q", wrk.namespace, wrk.bundleName, v)
		res := resourceMap[v]
		resClone, err := wrk.bp.scheme.DeepCopy(&res)
		if err != nil {
			return err
		}
		readyResource, err := wrk.checkResource(bundle, resClone.(*smith.Resource), readyResources)
		if err != nil {
			return err
		}
		log.Printf("[WORKER] bundle %s/%s: resource %q, ready: %t", wrk.namespace, wrk.bundleName, v, readyResource != nil)
		if readyResource != nil {
			readyResources[v] = readyResource
		} else {
			allReady = false
		}
	}

	// Updating the bundle state
	if allReady {
		return wrk.setBundleState(bundle, smith.READY)
	}
	if err := wrk.setBundleState(bundle, smith.IN_PROGRESS); err != nil {
		log.Printf("[WORKER] %v", err)
	}
	return nil
}

func (wrk *worker) checkResource(bundle *smith.Bundle, res *smith.Resource, readyResources map[smith.ResourceName]*unstructured.Unstructured) (readyResource *unstructured.Unstructured, e error) {
	// 1. Eval spec
	if err := wrk.evalSpec(bundle, res, readyResources); err != nil {
		return nil, err
	}

	// 2. Create or update resource
	resUpdated, err := wrk.createOrUpdate(res)
	if err != nil || resUpdated == nil {
		return nil, err
	}

	// 3. Check if resource is ready
	ready, err := wrk.bp.rc.IsReady(resUpdated)
	if err != nil || !ready {
		return nil, err
	}
	return resUpdated, nil
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
		bundle.Metadata.Labels,
		res.Spec.GetLabels(),
		map[string]string{smith.BundleNameLabel: wrk.bundleName}))

	// 3. Update OwnerReferences
	// Hardcode APIVersion/Kind because of https://github.com/kubernetes/client-go/issues/60
	// TODO uncomment when https://github.com/kubernetes/kubernetes/issues/39816 is fixed
	//res.Spec.SetOwnerReferences(append(res.Spec.GetOwnerReferences(), metav1.OwnerReference{
	//	APIVersion: smith.BundleResourceVersion,
	//	Kind:       smith.BundleResourceKind,
	//	Name:       bundle.Metadata.Name,
	//	UID:        bundle.Metadata.UID,
	//}))

	return nil
}

func (wrk *worker) createOrUpdate(res *smith.Resource) (*unstructured.Unstructured, error) {
	// 1. Prepare client
	gv, err := schema.ParseGroupVersion(res.Spec.GetAPIVersion())
	if err != nil {
		return nil, err
	}
	kind := res.Spec.GetKind()
	gvk := gv.WithKind(kind)
	client, err := wrk.bp.clients.ClientForGroupVersionKind(gvk)
	if err != nil {
		return nil, err
	}

	resClient := client.Resource(&metav1.APIResource{
		Name:       resources.ResourceKindToPath(kind),
		Namespaced: true,
		Kind:       kind,
	}, wrk.namespace)

	// 2. Try to get the resource. We do read first to avoid generating unnecessary events.
	obj, exists, err := wrk.bp.store.Get(gvk, wrk.namespace, res.Spec.GetName())
	if err != nil {
		// Unexpected error
		return nil, err
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
				return nil, err
			}
		}
	} else {
		log.Printf("[WORKER] bundle %s/%s: resource %q not found, creating", wrk.namespace, wrk.bundleName, res.Name)
		// 3. Create if does not exist
		response, err = resClient.Create(&res.Spec)
		if err == nil {
			log.Printf("[WORKER] bundle %s/%s: resource %q created", wrk.namespace, wrk.bundleName, res.Name)
			return response, nil
		}
		if errors.IsAlreadyExists(err) {
			log.Printf("[WORKER] bundle %s/%s: resource %q found, but not in Store yet", wrk.namespace, wrk.bundleName, res.Name)
			// We let the next rebuild() iteration, triggered by someone else creating the resource, to finish the work.
			return nil, nil
		}
		// Unexpected error
		return nil, err
	}

	// 4. Compare spec and existing resource
	updated, err := updateResource(wrk.bp.scheme, res, response)
	if err != nil {
		// Unexpected error
		return nil, err
	}
	if updated == nil {
		log.Printf("[WORKER] bundle %s/%s: resource %q has correct spec", wrk.namespace, wrk.bundleName, res.Name)
		return response, nil
	}

	// 5. Update if different
	response, err = resClient.Update(updated)
	if err != nil {
		if errors.IsConflict(err) {
			log.Printf("[WORKER] bundle %s/%s: resource %q update resulted in conflict, restarting loop", wrk.namespace, wrk.bundleName, res.Name)
			// We let the next rebuild() iteration, triggered by someone else creating the resource, to finish the work.
			return nil, nil
		}
		// Unexpected error
		return nil, err
	}
	log.Printf("[WORKER] bundle %s/%s: resource %q updated", wrk.namespace, wrk.bundleName, res.Name)
	return response, nil
}

func (wrk *worker) setBundleState(tpl *smith.Bundle, desired smith.ResourceState) error {
	if tpl.Status.State == desired {
		return nil
	}
	log.Printf("[WORKER] setting bundle %s/%s State to %q from %q", wrk.namespace, wrk.bundleName, desired, tpl.Status.State)
	tpl.Status.State = desired
	err := wrk.bp.bundleClient.Put().
		Namespace(wrk.namespace).
		Resource(smith.BundleResourcePath).
		Name(wrk.bundleName).
		Body(tpl).
		Do().
		Into(tpl)
	if err != nil {
		if kerrors.IsConflict(err) {
			// Something updated the bundle concurrently.
			// It is possible that it was us in previous iteration but we haven't observed the
			// resulting update event for the bundle and this iteration was triggered by something
			// else e.g. resource update.
			// It is safe to ignore this conflict because we will reiterate because of the update event.
			return nil
		}
		return fmt.Errorf("failed to set bundle %s/%s state to %q: %v", wrk.namespace, wrk.bundleName, desired, err)
	}
	log.Printf("[WORKER] bundle %s/%s is %q", wrk.namespace, wrk.bundleName, desired)
	return nil
}

// checkRebuild returns a pointer to the bundle if a rebuild is required.
// It returns nil if there is no new bundle and rebuild is not required.
func (wrk *worker) checkRebuild() *smith.Bundle {
	wrk.bp.lock.Lock()
	defer wrk.bp.lock.Unlock()
	bundle := wrk.bundle
	if bundle != nil {
		wrk.bundle = nil
		return bundle
	}
	// delete atomically with the check to avoid race with Processor's rebuildInternal()
	delete(wrk.bp.workers, wrk.workerRef)
	return nil
}

func (wrk *worker) cleanupState() {
	wrk.bp.lock.Lock()
	defer wrk.bp.lock.Unlock()
	if wrk.bp.workers[wrk.workerRef] == wrk {
		// Only cleanup if there is a stale reference to the current worker.
		delete(wrk.bp.workers, wrk.workerRef)
	}
}

func (wrk *worker) handleError(bundle *smith.Bundle, err error) (shouldContinue bool) {
	if err == context.Canceled || err == context.DeadlineExceeded {
		return false
	}
	log.Printf("[WORKER] Failed to rebuild the bundle %s/%s: %v", wrk.namespace, wrk.bundleName, err)
	next := wrk.bo.NextBackOff()
	if next == backoff.Stop {
		if e := wrk.setBundleState(bundle, smith.TERMINAL_ERROR); e != nil {
			log.Printf("[WORKER] %v", e)
		}
		return false
	}
	func() {
		wrk.bp.lock.Lock()
		defer wrk.bp.lock.Unlock()
		if wrk.bundle == nil { // Avoid overwriting bundle provided by external process
			wrk.bundle = bundle // Need to re-initialize wrk.bundle so that external loop continues to run
		}
	}()
	if e := wrk.setBundleState(bundle, smith.ERROR); e != nil {
		log.Printf("[WORKER] %v", e)
	}
	after := time.NewTimer(next)
	defer after.Stop()
	select {
	case <-wrk.bp.ctx.Done():
		return false
	case <-after.C:
	}
	return true
}

// checkNeedsRebuild can be called inside of the rebuild loop to check if the bundle needs to be rebuilt from the start.
func (wrk *worker) checkNeedsRebuild() bool {
	wrk.bp.lock.RLock()
	defer wrk.bp.lock.RUnlock()
	return wrk.bundle != nil
}

// updateResource checks if actual resource satisfies the desired spec.
// Returns non-nil object with updates applied or nil if actual matches desired.
func updateResource(scheme *runtime.Scheme, desired *smith.Resource, actual *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	spec := desired.Spec

	// TODO Handle deleted or to be deleted object. We need to wait for the object to be gone.

	upd, err := scheme.Copy(actual)
	if err != nil {
		return nil, err
	}
	updated := upd.(*unstructured.Unstructured)
	delete(updated.Object, "status")

	actClone, err := scheme.Copy(updated)
	if err != nil {
		return nil, err
	}
	actualClone := actClone.(*unstructured.Unstructured)

	// This is to ensure those fields actually exist in underlying map whether they are nil or empty slices/map
	actualClone.SetKind(actualClone.GetKind())
	actualClone.SetAPIVersion(actualClone.GetAPIVersion())
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
		valueClone, err := scheme.DeepCopy(value)
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
