package controller

import (
	"context"
	"fmt"
	"log"

	"github.com/atlassian/smith"
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	smithClient_v1 "github.com/atlassian/smith/pkg/client/clientset_generated/clientset/typed/smith/v1"
	"github.com/atlassian/smith/pkg/plugin"
	"github.com/atlassian/smith/pkg/util"
	"github.com/atlassian/smith/pkg/util/graph"

	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	"github.com/pkg/errors"
	core_v1 "k8s.io/api/core/v1"
	api_errors "k8s.io/apimachinery/pkg/api/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	unstructured_conversion "k8s.io/apimachinery/pkg/conversion/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
)

type syncTask struct {
	bundleClient   smithClient_v1.BundlesGetter
	smartClient    smith.SmartClient
	rc             ReadyChecker
	store          Store
	specCheck      SpecCheck
	bundle         *smith_v1.Bundle
	readyResources map[smith_v1.ResourceName]*unstructured.Unstructured
	plugins        map[smith_v1.PluginName]plugin.Plugin
	scheme         *runtime.Scheme
}

// Parse bundle, build resource graph, traverse graph, assert each resource exists.
// For each resource ensure its dependencies (if any) are in READY state before creating it.
// If at least one dependency is not READY - skip the resource. Rebuild will/should be called once the dependency
// updates it's state (noticed via watching).

// READY state might mean something different for each resource type. For a Custom Resource it may mean
// that a field "State" in the Status of the resource is set to "Ready". It is customizable via
// annotations with some defaults.
func (st *syncTask) process() (retriableError bool, e error) {
	// Build resource map by name
	resourceMap := make(map[smith_v1.ResourceName]smith_v1.Resource, len(st.bundle.Spec.Resources))
	for _, res := range st.bundle.Spec.Resources {
		if _, exist := resourceMap[res.Name]; exist {
			return false, errors.Errorf("bundle contains two resources with the same name %q", res.Name)
		}
		resourceMap[res.Name] = res
	}

	// Build the graph and topologically sort it
	g, sorted, sortErr := sortBundle(st.bundle)
	if sortErr != nil {
		return false, sortErr
	}

	st.readyResources = make(map[smith_v1.ResourceName]*unstructured.Unstructured, len(st.bundle.Spec.Resources))

	// Visit vertices in sorted order
nextVertex:
	for _, v := range sorted {
		// Check if all resource dependencies are ready (so we can start processing this one)
		for _, dependency := range g.Vertices[v].Edges() {
			if _, ok := st.readyResources[dependency.(smith_v1.ResourceName)]; !ok {
				log.Printf("[WORKER][%s/%s] Dependency %q is required by resource %q but it's not ready", st.bundle.Namespace, st.bundle.Name, dependency, v)
				continue nextVertex // Move to the next resource
			}
		}
		// Process the resource
		log.Printf("[WORKER][%s/%s] Checking resource %q", st.bundle.Namespace, st.bundle.Name, v)
		res := resourceMap[v.(smith_v1.ResourceName)]
		readyResource, retriable, err := st.checkResource(&res)
		if err != nil {
			return retriable, errors.Wrapf(err, "failed to process resource %q", res.Name)
		}
		log.Printf("[WORKER][%s/%s] Resource %q, ready: %t", st.bundle.Namespace, st.bundle.Name, v, readyResource != nil)
		if readyResource != nil {
			st.readyResources[v.(smith_v1.ResourceName)] = readyResource
		}
	}
	// Delete objects which were removed from the bundle
	retriable, err := st.deleteRemovedResources()
	if err != nil {
		return retriable, err
	}

	return false, nil
}

func (st *syncTask) checkResource(res *smith_v1.Resource) (readyResource *unstructured.Unstructured, retriableError bool, e error) {
	// 1. Eval spec
	spec, err := st.evalSpec(res)
	if err != nil {
		return nil, false, err
	}

	// 2. Create or update resource
	resUpdated, retriable, err := st.createOrUpdate(spec)
	if err != nil {
		return nil, retriable, err
	}

	// 3. Check if resource is ready
	ready, retriable, err := st.rc.IsReady(resUpdated)
	if err != nil || !ready {
		return nil, retriable, errors.Wrap(err, "readiness check failed")
	}
	return resUpdated, false, nil
}

// evalSpec evaluates the resource specification and returns the result.
func (st *syncTask) evalSpec(res *smith_v1.Resource) (*unstructured.Unstructured, error) {
	// 1. Process the spec
	var obj *unstructured.Unstructured
	var err error
	if res.Spec.Object != nil {
		obj, err = st.evalObjectSpec(res)
	} else if res.Spec.Plugin != nil {
		obj, err = st.evalPluginSpec(res)
	} else {
		return nil, errors.New("invalid resource")
	}

	if err != nil {
		return nil, err
	}

	// 2. Update label to point at the parent bundle
	obj.SetLabels(mergeLabels(
		st.bundle.Labels,
		obj.GetLabels(),
		map[string]string{smith.BundleNameLabel: st.bundle.Name}))

	// 3. Update OwnerReferences
	trueRef := true
	refs := obj.GetOwnerReferences()
	for i, ref := range refs {
		if ref.Controller != nil && *ref.Controller {
			return nil, errors.Errorf("cannot create resource with controller owner reference %v", ref)
		}
		refs[i].BlockOwnerDeletion = &trueRef
	}
	// Hardcode APIVersion/Kind because of https://github.com/kubernetes/client-go/issues/60
	refs = append(refs, meta_v1.OwnerReference{
		APIVersion:         smith_v1.BundleResourceGroupVersion,
		Kind:               smith_v1.BundleResourceKind,
		Name:               st.bundle.Name,
		UID:                st.bundle.UID,
		Controller:         &trueRef,
		BlockOwnerDeletion: &trueRef,
	})
	for _, dep := range res.DependsOn {
		obj := st.readyResources[dep] // this is ok because we've checked earlier that readyResources contains all dependencies
		refs = append(refs, meta_v1.OwnerReference{
			APIVersion:         obj.GetAPIVersion(),
			Kind:               obj.GetKind(),
			Name:               obj.GetName(),
			UID:                obj.GetUID(),
			BlockOwnerDeletion: &trueRef,
		})
	}
	obj.SetOwnerReferences(refs)

	return obj, nil
}

// evalPluginSpec evaluates the plugin resource specification and returns the result.
func (st *syncTask) evalPluginSpec(res *smith_v1.Resource) (*unstructured.Unstructured, error) {
	pluginInstance, ok := st.plugins[res.Spec.Plugin.Name]
	if !ok {
		return nil, errors.Errorf("no such plugin %q", res.Spec.Plugin.Name)
	}
	dependencies, err := st.prepareDependencies(res.DependsOn)
	if err != nil {
		return nil, err
	}
	log.Printf("Plugin %q dependencies: %+v", res.Spec.Plugin.Name, dependencies)

	result, err := pluginInstance.Process(res.Spec.Plugin.Spec, &plugin.Context{
		Dependencies: dependencies,
	})
	if err != nil {
		return nil, err
	}

	// Make sure plugin is returning us something that obeys the PluginSpec.
	object, err := util.RuntimeToUnstructured(result.Object)
	if err != nil {
		return nil, errors.Wrap(err, "plugin output cannot be converted from runtime.Object")
	}
	expectedGVK := pluginInstance.Describe().GVK
	if object.GroupVersionKind() != expectedGVK {
		return nil, errors.Errorf("unexpected GVK from plugin (wanted %v, got %v)", expectedGVK, object.GroupVersionKind())
	}
	// We are in charge of naming.
	object.SetName(res.Spec.Plugin.ObjectName)

	log.Printf("Plugin %q result: %+v", res.Spec.Plugin.Name, object)
	return object, nil
}

func (st *syncTask) prepareDependencies(dependsOn []smith_v1.ResourceName) (map[smith_v1.ResourceName]plugin.Dependency, error) {
	dependencies := make(map[smith_v1.ResourceName]plugin.Dependency, len(dependsOn))
	for _, name := range dependsOn {
		var dependency plugin.Dependency
		unstructuredActual := st.readyResources[name]
		gvk := unstructuredActual.GroupVersionKind()
		actual, err := st.scheme.New(gvk)
		if err != nil {
			return nil, err
		}
		if err = unstructured_conversion.DefaultConverter.FromUnstructured(unstructuredActual.Object, actual); err != nil {
			return nil, err
		}
		dependency.Actual = actual
		switch obj := actual.(type) {
		case *sc_v1b1.ServiceBinding:
			if err = st.prepareServiceBindingDependency(&dependency, obj); err != nil {
				return nil, errors.Wrapf(err, "error processing ServiceBinding %q", obj.Name)
			}
		}
		dependencies[name] = dependency
	}
	return dependencies, nil
}

func (st *syncTask) prepareServiceBindingDependency(dependency *plugin.Dependency, obj *sc_v1b1.ServiceBinding) error {
	secret, exists, err := st.store.Get(core_v1.SchemeGroupVersion.WithKind("Secret"), obj.Namespace, obj.Spec.SecretName)
	if err != nil {
		return errors.Wrap(err, "error finding output Secret")
	}
	if !exists {
		return errors.New("cannot find output Secret")
	}
	dependency.Outputs = append(dependency.Outputs, secret)

	serviceInstance, exists, err := st.store.Get(sc_v1b1.SchemeGroupVersion.WithKind("ServiceInstance"), obj.Namespace, obj.Spec.ServiceInstanceRef.Name)
	if err != nil {
		return errors.Wrapf(err, "error finding ServiceInstance %q", obj.Spec.ServiceInstanceRef.Name)
	}
	if !exists {
		return errors.Errorf("cannot find ServiceInstance %q", obj.Spec.ServiceInstanceRef.Name)
	}
	dependency.Auxiliary = append(dependency.Auxiliary, serviceInstance)
	return nil
}

// evalObjectSpec evaluates the regular resource specification and returns the result.
func (st *syncTask) evalObjectSpec(res *smith_v1.Resource) (*unstructured.Unstructured, error) {
	// Convert Spec to Unstructured
	spec, err := util.RuntimeToUnstructured(res.Spec.Object)

	if err != nil {
		return nil, err
	}

	// Process references
	sp := NewSpec(res.Name, st.readyResources, res.DependsOn)
	if err := sp.ProcessObject(spec.Object); err != nil {
		return nil, err
	}

	return spec, nil
}

// createOrUpdate creates or updates a resources.
func (st *syncTask) createOrUpdate(spec *unstructured.Unstructured) (actualRet *unstructured.Unstructured, retriableRet bool, e error) {
	// Prepare client
	gvk := spec.GroupVersionKind()
	resClient, err := st.smartClient.ForGVK(gvk, st.bundle.Namespace)
	if err != nil {
		return nil, false, errors.Wrapf(err, "failed to get the client for %q", gvk)
	}

	// Try to get the resource. We do read first to avoid generating unnecessary events.
	obj, exists, err := st.store.Get(gvk, st.bundle.Namespace, spec.GetName())
	if err != nil {
		// Unexpected error
		return nil, false, errors.Wrap(err, "failed to get object from the Store")
	}
	if exists {
		log.Printf("[WORKER][%s/%s] Object %s %q found, checking spec", st.bundle.Namespace, st.bundle.Name, gvk, spec.GetName())
		return st.updateResource(resClient, spec, obj)
	}
	log.Printf("[WORKER][%s/%s] Object %s %q not found, creating", st.bundle.Namespace, st.bundle.Name, gvk, spec.GetName())
	return st.createResource(resClient, spec)
}

func (st *syncTask) createResource(resClient dynamic.ResourceInterface, spec *unstructured.Unstructured) (actualRet *unstructured.Unstructured, retriableError bool, e error) {
	gvk := spec.GroupVersionKind()
	response, err := resClient.Create(spec)
	if err == nil {
		log.Printf("[WORKER][%s/%s] Object %s %q created", st.bundle.Namespace, st.bundle.Name, gvk, spec.GetName())
		return response, false, nil
	}
	if api_errors.IsAlreadyExists(err) {
		// We let the next processKey() iteration, triggered by someone else creating the resource, to finish the work.
		err = api_errors.NewConflict(schema.GroupResource{Group: gvk.Group, Resource: gvk.Kind}, spec.GetName(), err)
		return nil, false, errors.Wrapf(err, "object %q found, but not in Store yet (will re-process)", spec.GetName())
	}
	// Unexpected error, will retry
	return nil, true, err
}

// Mutates spec and actual.
func (st *syncTask) updateResource(resClient dynamic.ResourceInterface, spec *unstructured.Unstructured, actual runtime.Object) (actualRet *unstructured.Unstructured, retriableError bool, e error) {
	actualMeta := actual.(meta_v1.Object)
	// Check that the object is not marked for deletion
	if actualMeta.GetDeletionTimestamp() != nil {
		return nil, false, fmt.Errorf("object %v %q is marked for deletion", actual.GetObjectKind().GroupVersionKind(), actualMeta.GetName())
	}

	// Check that this bundle owns the object
	if !meta_v1.IsControlledBy(actualMeta, st.bundle) {
		return nil, false, fmt.Errorf("object %v %q is not owned by the Bundle", actual.GetObjectKind().GroupVersionKind(), actualMeta.GetName())
	}

	// Compare spec and existing resource
	updated, match, err := st.specCheck.CompareActualVsSpec(spec, actual)
	if err != nil {
		return nil, false, errors.Wrap(err, "specification check failed")
	}
	if match {
		log.Printf("[WORKER][%s/%s] Object %q has correct spec", st.bundle.Namespace, st.bundle.Name, spec.GetName())
		return updated, false, nil
	}

	// Update if different
	updated, err = resClient.Update(updated)
	if err != nil {
		if api_errors.IsConflict(err) {
			// We let the next processKey() iteration, triggered by someone else updating the resource, to finish the work.
			return nil, false, errors.Wrapf(err, "object %q update resulted in conflict (will re-process)", st.bundle.Namespace, st.bundle.Name, spec.GetName())
		}
		// Unexpected error, will retry
		return nil, true, err
	}
	log.Printf("[WORKER][%s/%s] Object %q updated", st.bundle.Namespace, st.bundle.Name, spec.GetName())
	return updated, false, nil
}

func (st *syncTask) deleteRemovedResources() (retriableError bool, e error) {
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
			// Invalid object, ignore
			continue
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

func (st *syncTask) setBundleStatus() error {
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
		return fmt.Errorf("failed to set bundle %s/%s status to %v: %v", st.bundle.Namespace, st.bundle.Name, st.bundle.Status.ShortString(), err)
	}
	log.Printf("[WORKER][%s/%s] Set bundle status to %s", st.bundle.Namespace, st.bundle.Name, bundleUpdated.Status.ShortString())
	return nil
}

func (st *syncTask) handleProcessResult(retriable bool, processErr error) (bool /*retriable*/, error) {
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

func (st *syncTask) isBundleReady() bool {
	for _, res := range st.bundle.Spec.Resources {
		if r := st.readyResources[res.Name]; r == nil {
			return false
		}
	}
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
