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
	"k8s.io/apimachinery/pkg/api/errors"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type worker struct {
	tp *BundleProcessor
	bo backoff.BackOff
	workerRef

	// The fields below are protected by tp.lock

	// It is ok to mutate the bundle itself, but pointer should only be updated while locked.
	bundle *smith.Bundle
}

func (wrk *worker) rebuildLoop() {
	defer wrk.tp.wg.Done()
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

// TODO parse bundle, build resource graph, traverse graph, assert each resource exists.
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
	resourceMap := make(map[string]smith.Resource, len(bundle.Spec.Resources))
	for _, res := range bundle.Spec.Resources {
		resourceMap[res.Name] = res
	}

	// Build the graph and topologically sort it
	graphData, sortErr := graph.TopologicalSort(bundle)
	if sortErr != nil {
		return sortErr
	}

	readyResources := make(map[string]struct{}, len(bundle.Spec.Resources))
	allReady := true

	// Visit vertices in sorted order
nextVertex:
	for _, v := range graphData.SortedVertices {
		log.Printf("[WORKER] bundle %s/%s: checking resource %s", wrk.namespace, wrk.bundleName, v)
		res := resourceMap[v]

		if wrk.checkNeedsRebuild() {
			return nil
		}
		resClone, err := wrk.tp.scheme.DeepCopy(&res)
		if err != nil {
			return err
		}
		// Check if all resource dependencies are ready (so we can start processing this one)
		for _, dependency := range graphData.Graph.Vertices[v].Edges() {
			if _, ok := readyResources[dependency]; !ok {
				allReady = false
				log.Printf("[WORKER] bundle %s/%s: dependencies are not ready for resource %s", wrk.namespace, wrk.bundleName, v)
				continue nextVertex // Move to the next resource
			}
		}
		// Process the resource
		isReady, err := wrk.checkResource(bundle, resClone.(*smith.Resource))
		if err != nil {
			return err
		}
		log.Printf("[WORKER] bundle %s/%s: resource %s, ready: %t", wrk.namespace, wrk.bundleName, v, isReady)
		if isReady {
			readyResources[v] = struct{}{}
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

func (wrk *worker) checkResource(bundle *smith.Bundle, res *smith.Resource) (isReady bool, e error) {
	gv, err := schema.ParseGroupVersion(res.Spec.GetAPIVersion())
	if err != nil {
		return false, err
	}
	kind := res.Spec.GetKind()
	client, err := wrk.tp.clients.ClientForGroupVersionKind(gv.WithKind(kind))
	if err != nil {
		return false, err
	}

	resClient := client.Resource(&metav1.APIResource{
		Name:       resources.ResourceKindToPath(kind),
		Namespaced: true,
		Kind:       kind,
	}, wrk.namespace)

	// 0. Update label to point at the parent bundle
	res.Spec.SetLabels(mergeLabels(
		bundle.Metadata.Labels,
		res.Spec.GetLabels(),
		map[string]string{smith.BundleNameLabel: wrk.bundleName}))

	// 1. Update OwnerReferences
	// Hardcode APIVersion/Kind because of https://github.com/kubernetes/client-go/issues/60
	// TODO uncomment when https://github.com/kubernetes/kubernetes/issues/39816 is fixed
	//res.Spec.SetOwnerReferences(append(res.Spec.GetOwnerReferences(), metav1.OwnerReference{
	//	APIVersion: smith.BundleResourceVersion,
	//	Kind:       smith.BundleResourceKind,
	//	Name:       bundle.Metadata.Name,
	//	UID:        bundle.Metadata.UID,
	//}))

	name := res.Spec.GetName()
	var response *unstructured.Unstructured
	for {
		// 2. Try to get the resource. We do read first to avoid generating unnecessary events.
		response, err = resClient.Get(name)
		if err != nil {
			if !errors.IsNotFound(err) {
				// Unexpected error
				return false, err
			}
			log.Printf("[WORKER] bundle %s/%s: resource %q not found, creating", wrk.namespace, wrk.bundleName, res.Name)
			// 3. Create if does not exist
			response, err = resClient.Create(&res.Spec)
			if err == nil {
				log.Printf("[WORKER] bundle %s/%s: resource %q created", wrk.namespace, wrk.bundleName, res.Name)
				break
			}
			if errors.IsAlreadyExists(err) {
				log.Printf("[WORKER] bundle %s/%s: resource %q found, restarting loop", wrk.namespace, wrk.bundleName, res.Name)
				continue
			}
			// Unexpected error
			return false, err
		}

		// 4. Compare spec and existing resource
		if isEqualResources(res, response) {
			//log.Printf("[WORKER] bundle %s/%s: resource %s has correct spec", wrk.namespace, wrk.bundleName, res.Name)
			break
		}

		// 5. Update if different
		// https://github.com/kubernetes/community/blob/master/contributors/devel/api-conventions.md#concurrency-control-and-consistency
		res.Spec.SetResourceVersion(response.GetResourceVersion()) // Do CAS

		response, err = resClient.Update(&res.Spec)
		if err != nil {
			if errors.IsConflict(err) {
				log.Printf("[WORKER] bundle %s/%s: resource %q update resulted in conflict, restarting loop", wrk.namespace, wrk.bundleName, res.Name)
				continue
			}
			// Unexpected error
			return false, err
		}
		log.Printf("[WORKER] bundle %s/%s: resource %q updated", wrk.namespace, wrk.bundleName, res.Name)
		break
	}
	return wrk.tp.rc.IsReady(response)
}

func (wrk *worker) setBundleState(tpl *smith.Bundle, desired smith.ResourceState) error {
	if tpl.Status.State == desired {
		return nil
	}
	log.Printf("[WORKER] setting bundle %s/%s State to %q from %q", wrk.namespace, wrk.bundleName, desired, tpl.Status.State)
	tpl.Status.State = desired
	err := wrk.tp.bundleClient.Put().
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
	// FIXME for some reason TypeMeta is not deserialized properly
	//log.Printf("[WORKER] Into = %#v", tpl)
	return nil
}

// checkRebuild returns a pointer to the bundle if a rebuild is required.
// It returns nil if there is no new bundle and rebuild is not required.
func (wrk *worker) checkRebuild() *smith.Bundle {
	wrk.tp.lock.Lock()
	defer wrk.tp.lock.Unlock()
	bundle := wrk.bundle
	if bundle != nil {
		wrk.bundle = nil
		return bundle
	}
	// delete atomically with the check to avoid race with Processor's rebuildInternal()
	delete(wrk.tp.workers, wrk.workerRef)
	return nil
}

func (wrk *worker) cleanupState() {
	wrk.tp.lock.Lock()
	defer wrk.tp.lock.Unlock()
	if wrk.tp.workers[wrk.workerRef] == wrk {
		// Only cleanup if there is a stale reference to the current worker.
		delete(wrk.tp.workers, wrk.workerRef)
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
		wrk.tp.lock.Lock()
		defer wrk.tp.lock.Unlock()
		wrk.bundle = bundle
	}()
	if e := wrk.setBundleState(bundle, smith.ERROR); e != nil {
		log.Printf("[WORKER] %v", e)
	}
	after := time.NewTimer(next)
	defer after.Stop()
	select {
	case <-wrk.tp.ctx.Done():
		return false
	case <-after.C:
	}
	return true
}

// needsRebuild can be called inside of the rebuild loop to check if the bundle needs to be rebuilt from the start.
func (wrk *worker) checkNeedsRebuild() bool {
	wrk.tp.lock.RLock()
	defer wrk.tp.lock.RUnlock()
	return wrk.bundle != nil
}

// isEqualResources checks that existing resource matches the desired spec.
func isEqualResources(res *smith.Resource, spec *unstructured.Unstructured) bool {
	// TODO implement
	// ignore additional annotations/labels? or make the merge behaviour configurable?
	return true
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
