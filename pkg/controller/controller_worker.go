package controller

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/atlassian/smith"
	"github.com/atlassian/smith/pkg/resources"
	"github.com/atlassian/smith/pkg/util/graph"

	"k8s.io/apimachinery/pkg/api/errors"
	api_errors "k8s.io/apimachinery/pkg/api/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
)

type objectRef struct {
	schema.GroupVersionKind
	Name string
}

func (c *BundleController) worker(ctx context.Context) {
	for c.processNextWorkItem() {
	}
}

func (c *BundleController) processNextWorkItem() bool {
	key, quit := c.queue.Get()
	if quit {
		return false
	}
	defer c.queue.Done(key)

	retriable, err := c.processKey(key.(string))
	c.handleErr(retriable, err, key)

	return true
}

func (c *BundleController) handleErr(retriable bool, err error, key interface{}) {
	if err == nil {
		c.queue.Forget(key)
		return
	}
	if retriable && c.queue.NumRequeues(key) < maxRetries {
		log.Printf("[WORKER][%s] Error syncing Bundle: %v", key, err)
		c.queue.AddRateLimited(key)
		return
	}

	log.Printf("[WORKER][%s] Dropping Bundle out of the queue: %v", key, err)
	c.queue.Forget(key)
}

func (c *BundleController) processKey(key string) (retriableRet bool, e error) {
	var conflict bool
	startTime := time.Now()
	log.Printf("[WORKER][%s] Started syncing Bundle", key)
	defer func() {
		msg := ""
		if conflict {
			msg = " (conflict)"
		}
		log.Printf("[WORKER][%s] Synced Bundle in %v%s", key, time.Now().Sub(startTime), msg)
	}()
	bundleObj, exists, err := c.bundleInf.GetIndexer().GetByKey(key)
	if err != nil {
		return false, err
	}
	if !exists {
		log.Printf("[WORKER][%s] Bundle has been deleted", key)
		return false, nil
	}

	// Deep-copy otherwise we are mutating our cache.
	bundleObjCopy, err := c.scheme.DeepCopy(bundleObj)
	if err != nil {
		return false, err
	}
	bundle := bundleObjCopy.(*smith.Bundle)
	var isReady, retriable bool
	isReady, conflict, retriable, err = c.process(bundle)
	if conflict {
		return false, nil
	}
	return c.handleProcessResult(bundle, isReady, retriable, err)
}

// Parse bundle, build resource graph, traverse graph, assert each resource exists.
// For each resource ensure its dependencies (if any) are in READY state before creating it.
// If at least one dependency is not READY, exit loop. Rebuild will/should be called once the dependency
// updates it's state (noticed via watching).

// READY state might mean something different for each resource type. For ThirdPartyResources it may mean
// that a field "State" in the Status of the resource is set to "Ready". It may be customizable via
// annotations with some defaults.
func (c *BundleController) process(bundle *smith.Bundle) (isReady, conflictRet, retriableError bool, e error) {
	// Build resource map by name
	resourceMap := make(map[smith.ResourceName]smith.Resource, len(bundle.Spec.Resources))
	for _, res := range bundle.Spec.Resources {
		if _, exist := resourceMap[res.Name]; exist {
			return false, false, false, fmt.Errorf("bundle contains two resources with the same name %q", res.Name)
		}
		resourceMap[res.Name] = res
	}

	// Build the graph and topologically sort it
	g, sorted, sortErr := sortBundle(bundle)
	if sortErr != nil {
		return false, false, false, sortErr
	}

	readyResources := make(map[smith.ResourceName]*unstructured.Unstructured, len(bundle.Spec.Resources))
	allReady := true

	// Visit vertices in sorted order
nextVertex:
	for _, v := range sorted {
		// Check if all resource dependencies are ready (so we can start processing this one)
		for _, dependency := range g.Vertices[v].Edges() {
			if _, ok := readyResources[dependency.(smith.ResourceName)]; !ok {
				allReady = false
				log.Printf("[WORKER][%s/%s] Dependency %q is required by resource %q but it's not ready", bundle.Namespace, bundle.Name, dependency, v)
				continue nextVertex // Move to the next resource
			}
		}
		// Process the resource
		log.Printf("[WORKER][%s/%s] Checking resource %q", bundle.Namespace, bundle.Name, v)
		res := resourceMap[v.(smith.ResourceName)]
		readyResource, conflict, retriable, err := c.checkResource(bundle, &res, readyResources)
		if err != nil || conflict {
			return false, conflict, retriable, err
		}
		log.Printf("[WORKER][%s/%s] Resource %q, ready: %t", bundle.Namespace, bundle.Name, v, readyResource != nil)
		if readyResource != nil {
			readyResources[v.(smith.ResourceName)] = readyResource
		} else {
			allReady = false
		}
	}
	// Delete objects which were removed from the bundle
	retriable, err := c.deleteRemovedResources(bundle)
	if err != nil {
		return false, false, retriable, err
	}

	return allReady, false, false, nil
}

func (c *BundleController) checkResource(bundle *smith.Bundle, res *smith.Resource, readyResources map[smith.ResourceName]*unstructured.Unstructured) (readyResource *unstructured.Unstructured, conflictRet, retriableError bool, e error) {
	// 1. Eval spec
	spec, err := c.evalSpec(bundle, res, readyResources)
	if err != nil {
		return nil, false, false, err
	}

	// 2. Create or update resource
	resUpdated, conflict, retriable, err := c.createOrUpdate(bundle, spec)
	if err != nil || conflict {
		return nil, conflict, retriable, err
	}

	// 3. Check if resource is ready
	ready, retriable, err := c.rc.IsReady(resUpdated)
	if err != nil || !ready {
		return nil, false, retriable, err
	}
	return resUpdated, false, false, nil
}

// evalSpec evaluates the resource specification and returns the result.
func (c *BundleController) evalSpec(bundle *smith.Bundle, res *smith.Resource, readyResources map[smith.ResourceName]*unstructured.Unstructured) (*unstructured.Unstructured, error) {
	// 0. Convert to Unstructured
	spec, err := res.ToUnstructured(c.scheme.DeepCopy)
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
		map[string]string{smith.BundleNameLabel: bundle.Name}))

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
	//spec.SetOwnerReferences(refs)
	setOwnerReferences(spec, refs)

	return spec, nil
}

// createOrUpdate creates or updates a resources.
// May return nil resource without any errors if an update/create conflict happened.
func (c *BundleController) createOrUpdate(bundle *smith.Bundle, spec *unstructured.Unstructured) (actualRet *unstructured.Unstructured, conflictRet, retriableRet bool, e error) {
	// Prepare client
	resClient, err := c.smartClient.ForGVK(spec.GroupVersionKind(), bundle.Namespace)
	if err != nil {
		return nil, false, false, err
	}
	gvk := spec.GetObjectKind().GroupVersionKind()

	// Try to get the resource. We do read first to avoid generating unnecessary events.
	obj, exists, err := c.store.Get(gvk, bundle.Namespace, spec.GetName())
	if err != nil {
		// Unexpected error
		return nil, false, false, err
	}
	if exists {
		return c.updateResource(bundle, resClient, spec, obj)
	}
	return c.createResource(bundle, resClient, spec)
}

func (c *BundleController) createResource(bundle *smith.Bundle, resClient *dynamic.ResourceClient, spec *unstructured.Unstructured) (actualRet *unstructured.Unstructured, conflictRet, retriableError bool, e error) {
	log.Printf("[WORKER][%s/%s] Object %q not found, creating", bundle.Namespace, bundle.Name, spec.GetName())
	response, err := resClient.Create(spec)
	if err == nil {
		log.Printf("[WORKER][%s/%s] Object %q created", bundle.Namespace, bundle.Name, spec.GetName())
		return response, false, false, nil
	}
	if errors.IsAlreadyExists(err) {
		log.Printf("[WORKER][%s/%s] Object %q found, but not in Store yet", bundle.Namespace, bundle.Name, spec.GetName())
		// We let the next rebuild() iteration, triggered by someone else creating the resource, to finish the work.
		return nil, true, false, nil
	}
	// Unexpected error, will retry
	return nil, false, true, err
}

// Mutates spec and actual.
func (c *BundleController) updateResource(bundle *smith.Bundle, resClient *dynamic.ResourceClient, spec *unstructured.Unstructured, actual runtime.Object) (actualRet *unstructured.Unstructured, conflictRet, retriableError bool, e error) {
	actualMeta := actual.(meta_v1.Object)
	// Check that the object is not marked for deletion
	if actualMeta.GetDeletionTimestamp() != nil {
		return nil, false, false, fmt.Errorf("object %v %q is marked for deletion", actual.GetObjectKind().GroupVersionKind(), actualMeta.GetName())
	}

	// Check that this bundle owns the object
	if !isOwner(actualMeta, bundle) {
		return nil, false, false, fmt.Errorf("object %v %q is not owned by the Bundle", actual.GetObjectKind().GroupVersionKind(), actualMeta.GetName())
	}

	// Compare spec and existing resource
	updated, match, err := c.specCheck.CompareActualVsSpec(spec, actual)
	if err != nil {
		return nil, false, false, err
	}
	if match {
		log.Printf("[WORKER][%s/%s] Object %q has correct spec", bundle.Namespace, bundle.Name, spec.GetName())
		return updated, false, false, nil
	}

	// Update if different
	updated, err = resClient.Update(updated)
	if err != nil {
		if errors.IsConflict(err) {
			log.Printf("[WORKER][%s/%s] Object %q update resulted in conflict, restarting loop", bundle.Namespace, bundle.Name, spec.GetName())
			// We let the next rebuild() iteration, triggered by someone else updating the resource, to finish the work.
			return nil, true, false, nil
		}
		// Unexpected error, will retry
		return nil, false, true, err
	}
	log.Printf("[WORKER][%s/%s] Object %q updated", bundle.Namespace, bundle.Name, spec.GetName())
	return updated, false, false, nil
}

func (c *BundleController) deleteRemovedResources(bundle *smith.Bundle) (retriableError bool, e error) {
	objs, err := c.store.GetObjectsForBundle(bundle.Namespace, bundle.Name)
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
		if !isOwner(m, bundle) {
			// Object is not owned by that bundle
			log.Printf("[WORKER][%s/%s] Object %v %q is not owned by the bundle with UID=%q. Owner references: %v",
				bundle.Namespace, bundle.Name, obj.GetObjectKind().GroupVersionKind(), m.GetName(), bundle.GetUID(), m.GetOwnerReferences())
			continue
		}
		ref := objectRef{
			GroupVersionKind: obj.GetObjectKind().GroupVersionKind(),
			Name:             m.GetName(),
		}
		existingObjs[ref] = m.GetUID()
	}
	for _, res := range bundle.Spec.Resources {
		m := res.Spec.(meta_v1.Object)
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
		log.Printf("[WORKER][%s/%s] Deleting object %v %q", bundle.Namespace, bundle.Name, ref.GroupVersionKind, ref.Name)
		resClient, err := c.smartClient.ForGVK(ref.GroupVersionKind, bundle.Namespace)
		if err != nil {
			if firstErr == nil {
				retriable = false
				firstErr = err
			} else {
				log.Printf("[WORKER][%s/%s] Failed to get client for object %s: %v", bundle.Namespace, bundle.Name, ref.GroupVersionKind, err)
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
				log.Printf("[WORKER][%s/%s] Failed to delete object %v %q: %v", bundle.Namespace, bundle.Name, ref.GroupVersionKind, ref.Name, err)
			}
			continue
		}
	}
	return retriable, firstErr
}

func (c *BundleController) setBundleStatus(bundle *smith.Bundle) error {
	err := c.bundleClient.Put().
		Namespace(bundle.Namespace).
		Resource(smith.BundleResourcePath).
		Name(bundle.Name).
		Body(bundle).
		Do().
		Into(bundle)
	if err != nil {
		if api_errors.IsConflict(err) {
			// Something updated the bundle concurrently.
			// It is possible that it was us in previous iteration but we haven't observed the
			// resulting update event for the bundle and this iteration was triggered by something
			// else e.g. resource update.
			// It is safe to ignore this conflict because we will reiterate because of the update event.
			return nil
		}
		return fmt.Errorf("failed to set bundle %s/%s status to %v: %v", bundle.Namespace, bundle.Name, bundle.Status.ShortString(), err)
	}
	log.Printf("[WORKER][%s/%s] Set bundle status to %s", bundle.Namespace, bundle.Name, bundle.Status.ShortString())
	return nil
}

func (c *BundleController) handleProcessResult(bundle *smith.Bundle, isReady, retriable bool, err error) (retriableRet bool, errRet error) {
	if err == context.Canceled || err == context.DeadlineExceeded {
		return false, err
	}
	inProgressCond := smith.BundleCondition{Type: smith.BundleInProgress, Status: smith.ConditionFalse}
	readyCond := smith.BundleCondition{Type: smith.BundleReady, Status: smith.ConditionFalse}
	errorCond := smith.BundleCondition{Type: smith.BundleError, Status: smith.ConditionFalse}
	if err == nil {
		if isReady {
			readyCond.Status = smith.ConditionTrue
		} else {
			inProgressCond.Status = smith.ConditionTrue
		}
	} else {
		errorCond.Status = smith.ConditionTrue
		errorCond.Message = err.Error()
		if retriable {
			errorCond.Reason = smith.BundleReasonRetriableError
			inProgressCond.Status = smith.ConditionTrue
		} else {
			errorCond.Reason = smith.BundleReasonTerminalError
		}
	}

	inProgressUpdated := bundle.UpdateCondition(&inProgressCond)
	readyUpdated := bundle.UpdateCondition(&readyCond)
	errorUpdated := bundle.UpdateCondition(&errorCond)

	// Updating the bundle state
	if inProgressUpdated || readyUpdated || errorUpdated {
		ex := c.setBundleStatus(bundle)
		if err == nil {
			err = ex
			retriable = true
		}
	}
	return retriable, err
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
	// Theoretically Bundle may be represented by multiple API versions, hence we only check name and UID.
	return ref != nil &&
		ref.Name == bundle.Name &&
		ref.UID == bundle.UID
}

func sortBundle(bundle *smith.Bundle) (*graph.Graph, []graph.V, error) {
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

// TODO remove the workaround below when https://github.com/kubernetes-incubator/service-catalog/pull/944 is merged
// and dependencies are updated.

func setNestedField(obj map[string]interface{}, value interface{}, fields ...string) {
	m := obj
	if len(fields) > 1 {
		for _, field := range fields[0 : len(fields)-1] {
			if _, ok := m[field].(map[string]interface{}); !ok {
				m[field] = make(map[string]interface{})
			}
			m = m[field].(map[string]interface{})
		}
	}
	m[fields[len(fields)-1]] = value
}

func setOwnerReference(src meta_v1.OwnerReference) map[string]interface{} {
	ret := make(map[string]interface{})
	setNestedField(ret, src.Kind, "kind")
	setNestedField(ret, src.Name, "name")
	setNestedField(ret, src.APIVersion, "apiVersion")
	setNestedField(ret, string(src.UID), "uid")
	// json.Unmarshal() extracts boolean json fields as bool, not as *bool and hence extractOwnerReference()
	// expects bool or a missing field, not *bool. So if pointer is nil, fields are omitted from the ret object.
	// If pointer is non-nil, they are set to the referenced value.
	if src.Controller != nil {
		setNestedField(ret, *src.Controller, "controller")
	}
	if src.BlockOwnerDeletion != nil {
		setNestedField(ret, *src.BlockOwnerDeletion, "blockOwnerDeletion")
	}
	return ret
}

func setOwnerReferences(u *unstructured.Unstructured, references []meta_v1.OwnerReference) {
	var newReferences = make([]map[string]interface{}, 0, len(references))
	for i := 0; i < len(references); i++ {
		newReferences = append(newReferences, setOwnerReference(references[i]))
	}
	if u.Object == nil {
		u.Object = make(map[string]interface{})
	}
	setNestedField(u.Object, newReferences, "metadata", "ownerReferences")
}
