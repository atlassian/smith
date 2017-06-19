package controller

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/atlassian/smith"
	"github.com/atlassian/smith/pkg/controller/graph"
	"github.com/atlassian/smith/pkg/resources"

	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	api_errors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	unstructured_conversion "k8s.io/apimachinery/pkg/conversion/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
)

var (
	converter = unstructured_conversion.NewConverter(false)
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
	startTime := time.Now()
	log.Printf("[WORKER][%s] Started syncing Bundle", key)
	defer func() {
		log.Printf("[WORKER][%s] Synced Bundle in %v", key, time.Now().Sub(startTime))
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
	bundleObjCopy, err := c.deepCopy(bundleObj)
	if err != nil {
		return false, err
	}
	bundle := bundleObjCopy.(*smith.Bundle)
	isReady, conflict, retriable, err := c.process(bundle)
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
	log.Printf("[WORKER][%s/%s] Rebuilding bundle", bundle.Namespace, bundle.Name)

	// Build resource map by name
	resourceMap := make(map[smith.ResourceName]smith.Resource, len(bundle.Spec.Resources))
	for _, res := range bundle.Spec.Resources {
		if _, exist := resourceMap[res.Name]; exist {
			return false, false, false, fmt.Errorf("bundle contains two resources with the same name %q", res.Name)
		}
		resourceMap[res.Name] = res
	}

	// Build the graph and topologically sort it
	graphData, sortErr := graph.TopologicalSort(bundle)
	if sortErr != nil {
		return false, false, false, sortErr
	}

	readyResources := make(map[smith.ResourceName]*unstructured.Unstructured, len(bundle.Spec.Resources))
	allReady := true

	// Visit vertices in sorted order
nextVertex:
	for _, v := range graphData.SortedVertices {
		// Check if all resource dependencies are ready (so we can start processing this one)
		for _, dependency := range graphData.Graph.Vertices[v].Edges() {
			if _, ok := readyResources[dependency]; !ok {
				allReady = false
				log.Printf("[WORKER][%s/%s] Dependency %q is required by resource %q but it's not ready", bundle.Namespace, bundle.Name, dependency, v)
				continue nextVertex // Move to the next resource
			}
		}
		// Process the resource
		log.Printf("[WORKER][%s/%s] Checking resource %q", bundle.Namespace, bundle.Name, v)
		res := resourceMap[v]
		readyResource, conflict, retriable, err := c.checkResource(bundle, &res, readyResources)
		if err != nil || conflict {
			return false, conflict, retriable, err
		}
		log.Printf("[WORKER][%s/%s] Resource %q, ready: %t", bundle.Namespace, bundle.Name, v, readyResource != nil)
		if readyResource != nil {
			readyResources[v] = readyResource
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
	spec, err := res.ToUnstructured(c.deepCopy)
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
func (c *BundleController) createOrUpdate(bundle *smith.Bundle, spec *unstructured.Unstructured) (resUpdated *unstructured.Unstructured, conflictRet, retriableError bool, e error) {
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

func (c *BundleController) createResource(bundle *smith.Bundle, resClient *dynamic.ResourceClient, spec *unstructured.Unstructured) (resUpdated *unstructured.Unstructured, conflictRet, retriableError bool, e error) {
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

func (c *BundleController) updateResource(bundle *smith.Bundle, resClient *dynamic.ResourceClient, spec *unstructured.Unstructured, obj runtime.Object) (resUpdated *unstructured.Unstructured, conflictRet, retriableError bool, e error) {
	existingObj, ok := obj.(*unstructured.Unstructured)
	if !ok {
		existingObj = &unstructured.Unstructured{
			Object: make(map[string]interface{}),
		}
		if err := converter.ToUnstructured(obj, &existingObj.Object); err != nil {
			// Unexpected error
			return nil, false, false, err
		}
	}

	// Check that the object is not marked for deletion
	if existingObj.GetDeletionTimestamp() != nil {
		return nil, false, false, fmt.Errorf("object %v %q is marked for deletion", existingObj.GroupVersionKind(), existingObj.GetName())
	}

	// Check that this bundle owns the object
	if !isOwner(existingObj, bundle) {
		return nil, false, false, fmt.Errorf("object %v %q is not owned by the Bundle", existingObj.GroupVersionKind(), existingObj.GetName())
	}

	// Compare spec and existing resource
	updated, err := updateResource(c.deepCopy, spec, existingObj)
	if err != nil {
		// Unexpected error
		return nil, false, false, err
	}
	if updated == nil {
		log.Printf("[WORKER][%s/%s] Object %q has correct spec", bundle.Namespace, bundle.Name, spec.GetName())
		return existingObj, false, false, nil
	}

	// Update if different
	existingObj, err = resClient.Update(updated)
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
	return existingObj, false, false, nil
}

func (c *BundleController) deleteRemovedResources(bundle *smith.Bundle) (retriableError bool, e error) {
	objs, err := c.store.GetObjectsForBundle(bundle.Namespace, bundle.Name)
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
	log.Printf("[WORKER][%s/%s] Setting bundle status to %s", bundle.Namespace, bundle.Name, bundle.Status.ShortString())
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
		return fmt.Errorf("failed to set bundle %s/%s status to %v: %v", bundle.Namespace, bundle.Name, bundle.Status, err)
	}
	return nil
}

func (c *BundleController) handleProcessResult(bundle *smith.Bundle, isReady, retriable bool, err error) (retriableRet bool, errRet error) {
	if err == context.Canceled || err == context.DeadlineExceeded {
		return false, err
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
		if retriable {
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
		ex := c.setBundleStatus(bundle)
		if err == nil {
			err = ex
			retriable = true
		}
	}
	return retriable, err
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
	//actualClone.SetOwnerReferences(actualClone.GetOwnerReferences())
	setOwnerReferences(actualClone, actualClone.GetOwnerReferences())
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
	//updated.SetOwnerReferences(spec.GetOwnerReferences()) // TODO Is this ok? Check that there is only one controller and it is THIS bundle
	setOwnerReferences(updated, spec.GetOwnerReferences())
	updated.SetFinalizers(spec.GetFinalizers()) // TODO Is this ok?

	// 3. Everything else
	for field, specValue := range spec.Object {
		switch field {
		case "kind", "apiVersion", "metadata":
			continue
		}
		specValueClone, err := deepCopy(specValue)
		if err != nil {
			return nil, err
		}
		updated.Object[field] = processField(specValueClone, updated.Object[field])
	}

	if !equality.Semantic.DeepEqual(updated, actualClone) {
		return updated, nil
	}
	return nil, nil
}

func processField(spec, actual interface{}) interface{} {
	specObj, ok := spec.(map[string]interface{})
	if !ok {
		return spec
	}
	actualObj, ok := actual.(map[string]interface{})
	if !ok {
		return spec
	}
	for field, specValue := range specObj {

	}
	return actualObj
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
