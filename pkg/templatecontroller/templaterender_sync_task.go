package templatecontroller

// TODO what we really want here is some shared 'reconciliation of objects'
// code between TemplateRender and Bundle, I think...
// But for now, copy/paste. It's the Go way! Also, innovation week.

import (
	"context"

	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	smithClient_v1 "github.com/atlassian/smith/pkg/client/clientset_generated/clientset/typed/smith/v1"
	"github.com/atlassian/smith/pkg/util/logz"

	"github.com/pkg/errors"
	"github.com/taskcluster/json-e"
	"go.uber.org/zap"
	api_errors "k8s.io/apimachinery/pkg/api/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/diff"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/cache"
)

type templateRenderSyncTask struct {
	logger               *zap.Logger
	smartClient          SmartClient
	templateInf          cache.SharedIndexInformer
	templateRenderClient smithClient_v1.TemplateRendersGetter
	store                Store
	specCheck            SpecCheck
}

// Parse bundle, build resource graph, traverse graph, assert each resource exists.
// For each resource ensure its dependencies (if any) are in READY state before creating it.
// If at least one dependency is not READY - skip the resource. Rebuild will/should be called once the dependency
// updates it's state (noticed via watching).

// READY state might mean something different for each resource type. For a Custom Resource it may mean
// that a field "State" in the Status of the resource is set to "Ready". It is customizable via
// annotations with some defaults.
func (trst *templateRenderSyncTask) process(templateRender *smith_v1.TemplateRender) (retriableError bool, e error) {
	// If the "deleteResources" finalizer is missing, add it and finish the processing iteration
	/* TODO
	if !hasDeleteResourcesFinalizer(st.bundle) {
		st.newFinalizers = addDeleteResourcesFinalizer(st.bundle.GetFinalizers())
		return false, nil
	}
	*/

	if templateRender.DeletionTimestamp != nil {
		return false, nil
	}

	// TODO fallback to some 'default' namespace if template not in current namespace
	// (i.e. allow user/dev overrides)
	// OR allow specification of namespace?
	// (either way, be careful of Smith's namespace restriction thing...)
	proposedObject, err := trst.renderProposedObject(templateRender)
	if err != nil {
		return false, err
	}

	// TODO force object name = template name?
	actualObject, err := trst.getActualObject(templateRender, proposedObject)
	if err != nil {
		return false, err
	}

	retriable, err := trst.processObject(templateRender.Namespace, proposedObject, actualObject)
	if err != nil && api_errors.IsConflict(errors.Cause(err)) {
		// Short circuit on conflict
		return retriable, err
	}
	return trst.handleProcessResult(templateRender, retriable, err)
}

func (trst *templateRenderSyncTask) renderProposedObject(templateRender *smith_v1.TemplateRender) (proposedObject *unstructured.Unstructured, e error) {
	templateKey := templateRender.ObjectMeta.Namespace + "/" + templateRender.Spec.TemplateName
	templateObj, exists, err := trst.templateInf.GetIndexer().GetByKey(templateKey)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get Template by key %q", templateKey)
	}
	if !exists {
		// TODO: this should set status
		return nil, errors.Errorf("no such Template %q", templateKey)
	}
	template := templateObj.(*smith_v1.Template)
	templateRenderSpec := templateRender.Spec.DeepCopy()
	object, err := jsone.Render(template.Spec, templateRenderSpec.Context)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to render template %s", templateRender.Name)
	}

	unstructuredObject, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&object)
	if err != nil {
		return nil, errors.Wrap(err, "can't convert RenderTemplate to unstructured after template application")
	}

	// TODO validate that unstructuredObject has GVK? Should be done by schema...
	u := unstructured.Unstructured{
		Object: unstructuredObject,
	}
	if u.GetName() != "" {
		return nil, errors.Errorf("name of object incorrectly defined by Template as %q", u.GetName())
	}
	if u.GetNamespace() != "" {
		return nil, errors.Errorf("namespace of object incorrectly defined by Template as %q", u.GetNamespace())
	}
	u.SetName(templateRender.Name)
	u.SetNamespace(templateRender.Namespace)
	// Update OwnerReferences
	trueRef := true
	refs := u.GetOwnerReferences()
	for i, ref := range refs {
		if ref.Controller != nil && *ref.Controller {
			return nil, errors.Errorf("cannot create resource with controller owner reference %v", ref)
		}
		refs[i].BlockOwnerDeletion = &trueRef
	}
	// Hardcode APIVersion/Kind because of https://github.com/kubernetes/client-go/issues/60
	refs = append(refs, meta_v1.OwnerReference{
		APIVersion:         smith_v1.TemplateRenderResourceGroupVersion,
		Kind:               smith_v1.TemplateRenderResourceKind,
		Name:               templateRender.Name,
		UID:                templateRender.UID,
		Controller:         &trueRef,
		BlockOwnerDeletion: &trueRef,
	})
	u.SetOwnerReferences(refs)

	return &u, nil
}

func (trst *templateRenderSyncTask) updateTemplateRender(render *smith_v1.TemplateRender) error {
	trst.logger.Sugar().Debugf("%+v", render)
	updated, err := trst.templateRenderClient.TemplateRenders(render.Namespace).Update(render)
	if err != nil {
		if api_errors.IsConflict(err) {
			// Something updated the bundle concurrently.
			// It is possible that it was us in previous iteration but we haven't observed the
			// resulting update event for the bundle and this iteration was triggered by something
			// else e.g. resource update.
			// It is safe to ignore this conflict because we will reiterate because of the update event.
			return nil
		}
		return errors.Wrap(err, "failed to update template with status")
	}
	trst.logger.Sugar().Debugf("Set template status to %s", &updated.Status)
	return nil
}

func (trst *templateRenderSyncTask) handleProcessResult(render *smith_v1.TemplateRender, retriable bool, processErr error) (bool /*retriable*/, error) {
	if processErr != nil && api_errors.IsConflict(errors.Cause(processErr)) {
		return retriable, processErr
	}
	if processErr == context.Canceled || processErr == context.DeadlineExceeded {
		return false, processErr
	}

	// TODO maybe just reflect the underlying object status back in most cases?
	// Actually, due to complexity of ReadyChecker, this is maybe not a good idea.
	// Should probably use ReadyChecker or ResourceStatus stuff?
	// Probably need to write own ready checker for this.
	// (unless internal error?)
	// (at the moment, this is a copy of bundle conditions, without the
	// attached processing...)
	/*
			render.Status.Conditions = []smith_v1.TemplateRenderCondition{
				{Type: smith_v1.TemplateRenderInProgress, Status: smith_v1.ConditionTrue},
				{Type: smith_v1.TemplateRenderReady, Status: smith_v1.ConditionFalse},
				{Type: smith_v1.TemplateRenderError, Status: smith_v1.ConditionFalse},
			}

		// TODO update only if necessary
		err := trst.updateTemplateRender(render)
		if err != nil {
			return true, err
		}
	*/

	return retriable, processErr
}

func (trst *templateRenderSyncTask) processObject(namespace string, proposed *unstructured.Unstructured, existing runtime.Object) (bool, error) {
	trst.logger.Debug("Processing object from template")

	// Try to get the resource. We do a read first to avoid generating unnecessary events.

	// Create or update resource
	resUpdated, retriable, err := trst.createOrUpdate(namespace, proposed, existing)
	if err != nil {
		return retriable, err
	}

	// Check if the resource actually matches the spec to detect infinite update cycles
	updatedSpec, match, err := trst.specCheck.CompareActualVsSpec(proposed, resUpdated)
	if err != nil {
		return false, errors.Wrap(err, "specification re-check failed")
	}
	if !match {
		trst.logger.Sugar().Warnf("Objects are different after specification re-check:\n%s",
			diff.ObjectReflectDiff(updatedSpec.Object, resUpdated.Object))
		return false, errors.New("specification of the created/updated object does not match the desired spec")
	}

	return false, nil
}

func (trst *templateRenderSyncTask) getActualObject(render *smith_v1.TemplateRender, obj *unstructured.Unstructured) (runtime.Object, error) {
	var gvk schema.GroupVersionKind
	var name string
	gvk = obj.GetObjectKind().GroupVersionKind()
	name = obj.GetName()
	actual, exists, err := trst.store.Get(gvk, render.Namespace, name)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get object from the Store")
	}
	if !exists {
		return nil, nil
	}
	actualMeta := actual.(meta_v1.Object)

	// Check that the object is not marked for deletion
	if actualMeta.GetDeletionTimestamp() != nil {
		return nil, errors.New("object is marked for deletion")
	}

	// Check that this bundle controls the object
	if !meta_v1.IsControlledBy(actualMeta, render) {
		ref := meta_v1.GetControllerOf(actualMeta)
		var err error
		if ref == nil {
			err = errors.New("object is not controlled by the TemplateRender and does not have a controller at all")
		} else {
			err = errors.Errorf("object is controlled by apiVersion=%s, kind=%s, name=%s, uid=%s, not by the TemplateRender (uid=%s)",
				ref.APIVersion, ref.Kind, ref.Name, ref.UID, render.UID)
		}
		return nil, err
	}
	return actual, nil
}

// createOrUpdate creates or updates a resources.
func (trst *templateRenderSyncTask) createOrUpdate(namespace string, spec *unstructured.Unstructured, actual runtime.Object) (actualRet *unstructured.Unstructured, retriableRet bool, e error) {
	// Prepare client
	gvk := spec.GroupVersionKind()
	resClient, err := trst.smartClient.ForGVK(gvk, namespace)
	if err != nil {
		return nil, false, errors.Wrapf(err, "failed to get the client for %q", gvk)
	}
	if actual != nil {
		trst.logger.Info("Object found, checking spec", logz.Gvk(gvk), logz.Object(spec))
		return trst.updateResource(resClient, spec, actual)
	}
	trst.logger.Info("Object not found, creating", logz.Gvk(gvk), logz.Object(spec))
	return trst.createResource(resClient, spec)
}

func (trst *templateRenderSyncTask) createResource(resClient dynamic.ResourceInterface, spec *unstructured.Unstructured) (actualRet *unstructured.Unstructured, retriableError bool, e error) {
	gvk := spec.GroupVersionKind()
	response, err := resClient.Create(spec)
	if err == nil {
		trst.logger.Info("Object created", logz.Gvk(gvk), logz.Object(spec))
		return response, false, nil
	}
	if api_errors.IsAlreadyExists(err) {
		// We let the next ProcessKey() iteration, triggered by someone else creating the resource, to finish the work.
		err = api_errors.NewConflict(schema.GroupResource{Group: gvk.Group, Resource: gvk.Kind}, spec.GetName(), err)
		return nil, false, errors.Wrap(err, "object found, but not in Store yet (will re-process)")
	}
	// Unexpected error, will retry
	return nil, true, err
}

// Mutates spec and actual.
func (trst *templateRenderSyncTask) updateResource(resClient dynamic.ResourceInterface, spec *unstructured.Unstructured, actual runtime.Object) (actualRet *unstructured.Unstructured, retriableError bool, e error) {
	// Compare spec and existing resource
	updated, match, err := trst.specCheck.CompareActualVsSpec(spec, actual)
	if err != nil {
		return nil, false, errors.Wrap(err, "specification check failed")
	}
	if match {
		trst.logger.Info("Object has correct spec", logz.Object(spec))
		return updated, false, nil
	}

	// Update if different
	updated, err = resClient.Update(updated)
	if err != nil {
		if api_errors.IsConflict(err) {
			// We let the next ProcessKey() iteration, triggered by someone else updating the resource, finish the work.
			return nil, false, errors.Wrap(err, "object update resulted in conflict (will re-process)")
		}
		// Unexpected error, will retry
		return nil, true, err
	}
	trst.logger.Info("Object updated", logz.Object(spec))
	return updated, false, nil
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
