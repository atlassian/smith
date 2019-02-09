package bundlec

import (
	ctrlLogz "github.com/atlassian/ctrl/logz"
	"github.com/atlassian/smith"
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/smith/pkg/plugin"
	"github.com/atlassian/smith/pkg/statuschecker"
	"github.com/atlassian/smith/pkg/store"
	"github.com/atlassian/smith/pkg/util"
	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	core_v1 "k8s.io/api/core/v1"
	api_errors "k8s.io/apimachinery/pkg/api/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/diff"
	k8s_errors "k8s.io/apimachinery/pkg/util/errors"
	k8s_json "k8s.io/apimachinery/pkg/util/json"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/dynamic"
)

type ResourceStatusType string

const (
	ResourceStatusTypeReady                ResourceStatusType = "Ready"
	ResourceStatusTypeInProgress           ResourceStatusType = "InProgress"
	ResourceStatusTypeDependenciesNotReady ResourceStatusType = "DependenciesNotReady"
	ResourceStatusTypeError                ResourceStatusType = "Error"
)

var (
	isCreateOrUpdateInternal = [...]func(error) bool{
		// AlreadyExists only happens during the create call, and we catch it
		// and throw a Conflict. Should not happen.
		api_errors.IsAlreadyExists,

		// Expired is typically only associated with Watches and this shouldn't
		// happen at all
		api_errors.IsResourceExpired,

		// Shouldn't happen at all because the method should always be right
		api_errors.IsMethodNotSupported,

		// Shouldn't happen at all because the accept types should be right
		api_errors.IsNotAcceptable,

		// Shouldn't happen at all because the client sets Content-Type.
		api_errors.IsUnsupportedMediaType,

		// BadRequest is different from Invalid - this indicates the created
		// request itself is wrong and doesn't make sense.
		api_errors.IsBadRequest,
	}

	isCreateOrUpdateNonRetriable = [...]func(error) bool{
		// Conflict indicates that the object will be requeued, so we can avoid
		// the retry
		api_errors.IsConflict,

		// Invalid indicates the object itself is wrong, and we should probably
		// just make this a terminal error
		api_errors.IsInvalid,
	}

	prohibitedAnnotations = sets.NewString(smith.DeletionTimestampAnnotation)
)

// resourceStatus is one of "resourceStatus*" structs.
// It is a mechanism to communicate the status of a resource.
type resourceStatus interface {
	StatusType() ResourceStatusType
}

// resourceStatusDependenciesNotReady means resource processing is blocked by dependencies that are not ready.
type resourceStatusDependenciesNotReady struct {
	dependencies []smith_v1.ResourceName
}

// resourceStatusInProgress means resource is being processed by its controller.
type resourceStatusInProgress struct {
	message string
}

// resourceStatusReady means resource is ready.
type resourceStatusReady struct {
	message string
}

// resourceStatusError means there was an error processing this resource.
type resourceStatusError struct {
	err              error
	isRetriableError bool
	isExternalError  bool
}

func (r resourceStatusReady) StatusType() ResourceStatusType {
	return ResourceStatusTypeReady
}
func (r resourceStatusDependenciesNotReady) StatusType() ResourceStatusType {
	return ResourceStatusTypeDependenciesNotReady
}
func (r resourceStatusInProgress) StatusType() ResourceStatusType {
	return ResourceStatusTypeInProgress
}
func (r resourceStatusError) StatusType() ResourceStatusType {
	return ResourceStatusTypeError
}

type resourceInfo struct {
	actual *unstructured.Unstructured
	status resourceStatus

	// if actual is a ServiceBinding, we resolve the secret once it's been processed.
	serviceBindingSecret *core_v1.Secret
}

func (ri *resourceInfo) isReady() bool {
	_, ok := ri.status.(resourceStatusReady)
	return ok
}

func (ri *resourceInfo) fetchError() *resourceStatusError {
	if rse, ok := ri.status.(resourceStatusError); ok {
		return &rse
	}
	return nil
}

type resourceSyncTask struct {
	logger             *zap.Logger
	smartClient        SmartClient
	checker            statuschecker.Interface
	store              Store
	specChecker        SpecChecker
	bundle             *smith_v1.Bundle
	processedResources map[smith_v1.ResourceName]*resourceInfo
	pluginContainers   map[smith_v1.PluginName]plugin.Container
	scheme             *runtime.Scheme
	catalog            *store.Catalog
}

func (st *resourceSyncTask) processResource(res *smith_v1.Resource) resourceInfo {
	st.logger.Debug("Processing resource")

	// Do as much prevalidation of the spec as we can before dependencies are resolved.
	// (e.g. plugin/service instance/service binding schemas)
	// We may want to move this out of the resource processing entirely and do
	// it before we start processing the bundle, both to fail faster and avoid
	// some unnecessary schema validations (particularly if we want to cache
	// entire bundle prevalidation results rather than specific validations).
	status := st.prevalidate(res)
	if status != nil {
		return resourceInfo{
			status: status,
		}
	}

	// Check if all resource dependencies are ready (so we can start processing this one)
	notReadyDependencies := st.checkAllDependenciesAreReady(res)
	if len(notReadyDependencies) > 0 {
		st.logger.Sugar().Infof("Dependencies required by resource but not ready: %q", notReadyDependencies)
		return resourceInfo{
			status: resourceStatusDependenciesNotReady{
				dependencies: notReadyDependencies,
			},
		}
	}

	// Try to get the resource. We do a read first to avoid generating unnecessary events.
	actual, status := st.getActualObject(res)
	if status != nil {
		return resourceInfo{
			status: status,
		}
	}

	// Eval spec
	spec, status := st.evalSpec(res, actual)
	if status != nil {
		return resourceInfo{
			status: status,
		}
	}

	// Validate spec
	status = st.validateSpec(spec)
	if status != nil {
		return resourceInfo{
			status: status,
		}
	}
	if status != nil {
		return resourceInfo{
			status: status,
		}
	}

	// Create or update resource
	resUpdated, retriable, err := st.createOrUpdate(spec, actual)
	if err != nil {
		cause := errors.Cause(err)

		for _, isInternalErr := range isCreateOrUpdateInternal {
			if isInternalErr(cause) {
				return resourceInfo{
					actual: resUpdated,
					status: resourceStatusError{
						err: err,
					},
				}

			}
		}
		for _, isNonRetriable := range isCreateOrUpdateNonRetriable {
			if isNonRetriable(cause) {
				return resourceInfo{
					actual: resUpdated,
					status: resourceStatusError{
						err:             err,
						isExternalError: true,
					},
				}
			}
		}

		return resourceInfo{
			actual: resUpdated,
			status: resourceStatusError{
				err:              err,
				isRetriableError: retriable,
				isExternalError:  true,
			},
		}
	}

	// Check if the resource actually matches the spec to detect infinite update cycles
	updatedSpec, match, _, err := st.specChecker.CompareActualVsSpec(st.logger, spec, resUpdated)

	switch {
	case err != nil:
		return resourceInfo{
			status: resourceStatusError{
				err: err,
			},
		}
	case !match && util.IsSecret(updatedSpec):
		// Don't log the secret
		st.logger.Error("Objects are different after specification re-check: Secret object does not match")
		return resourceInfo{
			status: resourceStatusError{
				err: errors.New("specification of the created/updated object does not match the desired spec"),
			},
		}
	case !match:
		// We use reflect diff here instead of the returned json diff to see the types
		difference := diff.ObjectReflectDiff(updatedSpec.Object, resUpdated.Object)
		st.logger.Sugar().Errorf("Objects are different after specification re-check (`a` is what we've sent and `b` is what Kubernetes persisted and returned):\n%s", difference)
		return resourceInfo{
			status: resourceStatusError{
				err: errors.New("specification of the created/updated object does not match the desired spec"),
			},
		}
	}

	// Check if resource is ready
	statusResult := st.checker.CheckStatus(resUpdated)
	switch s := statusResult.(type) {
	case statuschecker.ObjectStatusInProgress:
		return resourceInfo{
			actual: resUpdated,
			status: resourceStatusInProgress{
				message: s.Message,
			},
		}
	case statuschecker.ObjectStatusError:
		return resourceInfo{
			actual: resUpdated,
			status: resourceStatusError{
				err:              s.Error,
				isRetriableError: s.RetriableError,
				isExternalError:  s.ExternalError,
			},
		}
	case statuschecker.ObjectStatusUnknown:
		return resourceInfo{
			actual: resUpdated,
			status: resourceStatusError{
				err: errors.Errorf("unknown status: %v", s.Details),
			},
		}
	case statuschecker.ObjectStatusReady:
		// Augment with binding output (used for references)
		bindingSecret, err := st.maybeExtractBindingSecret(resUpdated)
		if err != nil {
			return resourceInfo{
				actual: resUpdated,
				status: resourceStatusError{
					err: err,
				},
			}
		}

		return resourceInfo{
			actual: resUpdated,
			status: resourceStatusReady{
				message: s.Message,
			},
			serviceBindingSecret: bindingSecret,
		}
	default:
		return resourceInfo{
			actual: resUpdated,
			status: resourceStatusError{
				err: errors.Errorf("unknown ObjectStatus %q", s.StatusType()),
			},
		}
	}
}

func (st *resourceSyncTask) maybeExtractBindingSecret(obj *unstructured.Unstructured) (*core_v1.Secret, error) {
	if obj.GroupVersionKind() != sc_v1b1.SchemeGroupVersion.WithKind("ServiceBinding") {
		return nil, nil
	}
	actual, err := st.scheme.ConvertToVersion(obj, sc_v1b1.SchemeGroupVersion)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	serviceBinding := actual.(*sc_v1b1.ServiceBinding)
	secret, exists, err := st.store.Get(core_v1.SchemeGroupVersion.WithKind("Secret"), serviceBinding.Namespace, serviceBinding.Spec.SecretName)
	if err != nil {
		return nil, errors.Wrap(err, "error finding output Secret")
	}
	if !exists {
		return nil, errors.New("cannot find output Secret")
	}
	return secret.(*core_v1.Secret), nil
}

func (st *resourceSyncTask) checkAllDependenciesAreReady(res *smith_v1.Resource) []smith_v1.ResourceName {
	// No len here because dependencies can occur more than once in reference list
	notReadyDependenciesSet := make(map[smith_v1.ResourceName]struct{})
	for _, reference := range res.References {
		if !st.processedResources[reference.Resource].isReady() {
			notReadyDependenciesSet[reference.Resource] = struct{}{}
		}
	}
	notReadyDependencies := make([]smith_v1.ResourceName, 0, len(notReadyDependenciesSet))
	for resourceName := range notReadyDependenciesSet {
		notReadyDependencies = append(notReadyDependencies, resourceName)
	}
	return notReadyDependencies
}

func (st *resourceSyncTask) getActualObject(res *smith_v1.Resource) (runtime.Object, resourceStatus) {
	var gvk schema.GroupVersionKind
	var name string

	switch {
	case res.Spec.Object != nil:
		gvk = res.Spec.Object.GetObjectKind().GroupVersionKind()
		name = res.Spec.Object.(meta_v1.Object).GetName()
	case res.Spec.Plugin != nil:
		pluginContainer, ok := st.pluginContainers[res.Spec.Plugin.Name]
		if !ok {
			return nil, resourceStatusError{
				err:             errors.Errorf("no such plugin %q", res.Spec.Plugin.Name),
				isExternalError: true,
			}
		}
		gvk = pluginContainer.Plugin.Describe().GVK
		name = res.Spec.Plugin.ObjectName
	default:
		// unreachable
		return nil, resourceStatusError{
			err:             errors.New(`neither "object" nor "plugin" field is specified`),
			isExternalError: true,
		}
	}
	actual, exists, err := st.store.Get(gvk, st.bundle.Namespace, name)
	if err != nil {
		// internal error - something is up with our stores
		return nil, resourceStatusError{
			err: errors.Wrap(err, "failed to get object from the Store"),
		}
	}
	if !exists {
		return nil, nil
	}
	actualMeta := actual.(meta_v1.Object)

	// Check that the object is not marked for deletion
	if actualMeta.GetDeletionTimestamp() != nil {
		return nil, resourceStatusError{
			err:             errors.New("object is marked for deletion"),
			isExternalError: true,
		}
	}

	// Check that this bundle controls the object
	if !meta_v1.IsControlledBy(actualMeta, st.bundle) {
		ref := meta_v1.GetControllerOf(actualMeta)
		var err error
		if ref == nil {
			err = errors.New("object is not controlled by the Bundle and does not have a controller at all")
		} else {
			err = errors.Errorf("object is controlled by apiVersion=%s, kind=%s, name=%s, uid=%s, not by the Bundle (uid=%s)",
				ref.APIVersion, ref.Kind, ref.Name, ref.UID, st.bundle.UID)
		}
		return nil, resourceStatusError{
			err:             err,
			isExternalError: true,
		}
	}
	return actual, nil
}

// prevalidate does as much validation as possible before doing any real work.
func (st *resourceSyncTask) prevalidate(res *smith_v1.Resource) resourceStatus {
	sp, err := newExamplesSpec(res.References)
	if err != nil {
		if isNoExampleError(errors.Cause(err)) {
			// a noExampleError occurs when an example wasn't provided
			// by the user in one of the references. For now, we assume this
			// is intentional and don't error out.
			st.logger.Debug("Not validating against schema due to missing examples", zap.Error(err))
			return nil
		}
		return resourceStatusError{err: err}
	}
	serviceInstanceGvk := sc_v1b1.SchemeGroupVersion.WithKind("ServiceInstance")
	res = res.DeepCopy() // Spec processor mutates in place

	if res.Spec.Object != nil {
		if res.Spec.Object.GetObjectKind().GroupVersionKind() == serviceInstanceGvk {
			if st.catalog == nil {
				// can't do anything, since service catalog wasn't enabled.
				return nil
			}
			actual, err := st.scheme.ConvertToVersion(res.Spec.Object, serviceInstanceGvk.GroupVersion())
			if err != nil {
				return resourceStatusError{err: errors.WithStack(err)}
			}
			serviceInstance := actual.(*sc_v1b1.ServiceInstance)

			if len(serviceInstance.Spec.ParametersFrom) > 0 {
				st.logger.Debug("Not validating against schema due to parametersFrom block")
				return nil
			}

			if serviceInstance.Spec.Parameters != nil {
				var parameters map[string]interface{}
				if err = k8s_json.Unmarshal(serviceInstance.Spec.Parameters.Raw, &parameters); err != nil {
					return resourceStatusError{
						err:             errors.Wrap(err, "unable to unmarshal ServiceInstance resource parameters as object"),
						isExternalError: true,
					}
				}

				if err = sp.ProcessObject(parameters); err != nil {
					return resourceStatusError{
						err:             err,
						isExternalError: true,
					}
				}

				serviceInstance.Spec.Parameters.Raw, err = k8s_json.Marshal(parameters)
				if err != nil {
					return resourceStatusError{err: errors.WithStack(err)}
				}
			}

			validationResult, err := st.catalog.ValidateServiceInstanceSpec(&serviceInstance.Spec)
			if err != nil {
				return resourceStatusError{err: err}
			}
			if len(validationResult.Errors) > 0 {
				return resourceStatusError{
					err:             errors.Wrap(k8s_errors.NewAggregate(validationResult.Errors), "spec failed validation against schema"),
					isExternalError: true,
				}
			}
		}
		// TODO validate service binding parameters
		// (low priority, not currently used)
	} else if res.Spec.Plugin != nil {
		if res.Spec.Plugin.Spec != nil {
			if err := sp.ProcessObject(res.Spec.Plugin.Spec); err != nil {
				return resourceStatusError{
					err:             err,
					isExternalError: true,
				}
			}
		}
		pluginContainer, ok := st.pluginContainers[res.Spec.Plugin.Name]
		if !ok {
			return resourceStatusError{
				err:             errors.Errorf("plugin %q does not exist", res.Spec.Plugin.Name),
				isExternalError: true,
			}
		}
		validationResult, err := pluginContainer.ValidateSpec(res.Spec.Plugin.Spec)
		if err != nil {
			return resourceStatusError{err: err}
		}
		if len(validationResult.Errors) > 0 {
			return resourceStatusError{
				err:             errors.Wrap(k8s_errors.NewAggregate(validationResult.Errors), "spec failed validation against schema"),
				isExternalError: true,
			}
		}
	}

	return nil
}

// evalSpec evaluates the resource specification and returns the result.
func (st *resourceSyncTask) evalSpec(res *smith_v1.Resource, actual runtime.Object) (*unstructured.Unstructured, resourceStatus) {
	// Process the spec
	var objectOrPluginSpec map[string]interface{}

	switch {
	case res.Spec.Object != nil:
		specUnstr, err := util.RuntimeToUnstructured(res.Spec.Object)
		if err != nil {
			return nil, resourceStatusError{
				err: err,
			}
		}
		objectOrPluginSpec = specUnstr.Object
	case res.Spec.Plugin != nil:
		res = res.DeepCopy() // Spec processor mutates in place
		objectOrPluginSpec = res.Spec.Plugin.Spec
	default:
		return nil, resourceStatusError{
			err:             errors.New(`neither "object" nor "plugin" field is specified`),
			isExternalError: true,
		}
	}

	// Process references
	sp, err := newSpec(st.processedResources, res.References)
	if err != nil {
		return nil, resourceStatusError{
			err:             err,
			isExternalError: true,
		}
	}
	if err := sp.ProcessObject(objectOrPluginSpec); err != nil {
		return nil, resourceStatusError{
			err:             err,
			isExternalError: true,
		}
	}

	var obj *unstructured.Unstructured
	switch {
	case res.Spec.Object != nil:
		obj = &unstructured.Unstructured{
			Object: objectOrPluginSpec,
		}
	case res.Spec.Plugin != nil:
		var status resourceStatus
		obj, status = st.evalPluginSpec(res, actual)
		if status != nil {
			return nil, status
		}
	default:
		return nil, resourceStatusError{
			err:             errors.New(`neither "object" nor "plugin" field is specified`),
			isExternalError: true,
		}
	}

	// Update OwnerReferences
	trueRef := true
	refs := obj.GetOwnerReferences()
	for i, ref := range refs {
		if ref.Controller != nil && *ref.Controller {
			// user (or plugin) tried to create a resource with controller owner reference
			return nil, resourceStatusError{
				err:             errors.Errorf("cannot create resource with controller owner reference %v", ref),
				isExternalError: res.Spec.Plugin == nil,
			}
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
	setRefs := make(map[smith_v1.ResourceName]struct{}, len(res.References))
	for _, dep := range res.References {
		if _, ok := setRefs[dep.Resource]; ok {
			// This resource is referenced more than once and an owner reference has been added already
			continue
		}
		setRefs[dep.Resource] = struct{}{}
		processedObj := st.processedResources[dep.Resource].actual // this is ok because we've checked earlier that resources contains all dependencies
		refs = append(refs, meta_v1.OwnerReference{
			APIVersion:         processedObj.GetAPIVersion(),
			Kind:               processedObj.GetKind(),
			Name:               processedObj.GetName(),
			UID:                processedObj.GetUID(),
			BlockOwnerDeletion: &trueRef,
		})
	}
	obj.SetOwnerReferences(refs)
	switch obj.GetNamespace() {
	case "":
		obj.SetNamespace(st.bundle.Namespace)
	case st.bundle.Namespace:
	default:
		// the plugin or user created an object template with the namespace set to something wrong
		return nil, resourceStatusError{
			err:             errors.Errorf("namespace was %q which is different from the bundle namespace %q", obj.GetNamespace(), st.bundle.Namespace),
			isExternalError: res.Spec.Plugin == nil,
		}
	}

	return obj, nil
}

// validateSpec enforces constraints on the desires object spec
// e.g. prohibits Smith-managed annotations
func (st *resourceSyncTask) validateSpec(spec *unstructured.Unstructured) resourceStatus {
	annotations := spec.GetAnnotations()
	if len(annotations) > 0 {
		for key := range prohibitedAnnotations {
			if _, ok := annotations[key]; ok {
				return resourceStatusError{
					err:              errors.Errorf("annotation %q cannot be set by the user", key),
					isRetriableError: false,
				}
			}
		}
	}
	return nil
}

// evalPluginSpec evaluates the plugin resource specification and returns the result.
func (st *resourceSyncTask) evalPluginSpec(res *smith_v1.Resource, actual runtime.Object) (*unstructured.Unstructured, resourceStatus) {
	pluginContainer, ok := st.pluginContainers[res.Spec.Plugin.Name]
	if !ok {
		return nil, resourceStatusError{
			err:             errors.Errorf("no such plugin %q", res.Spec.Plugin.Name),
			isExternalError: true,
		}
	}
	validationResult, err := pluginContainer.ValidateSpec(res.Spec.Plugin.Spec)
	if err != nil {
		return nil, resourceStatusError{err: err}
	}
	if len(validationResult.Errors) > 0 {
		return nil, resourceStatusError{
			err:             errors.Wrap(k8s_errors.NewAggregate(validationResult.Errors), "spec failed validation against schema"),
			isExternalError: true,
		}
	}

	// validate above should guarantee that our plugin is there
	dependencies, err := st.prepareDependencies(res.References)
	if err != nil {
		// there should be no error in processing dependencies. If there is, this
		// is an internal issue.
		return nil, resourceStatusError{
			err: err,
		}
	}

	result := pluginContainer.Plugin.Process(res.Spec.Plugin.Spec, &plugin.Context{
		Namespace:    st.bundle.Namespace,
		Actual:       actual,
		Dependencies: dependencies,
	})
	var pluginObj runtime.Object
	switch res := result.(type) {
	case *plugin.ProcessResultSuccess:
		pluginObj = res.Object
	case *plugin.ProcessResultFailure:
		return nil, resourceStatusError{
			err:              res.Error,
			isRetriableError: res.IsRetriableError,
			isExternalError:  res.IsExternalError,
		}
	default:
		return nil, resourceStatusError{
			err: errors.Errorf("unexpected plugin result type %q", res.StatusType()),
		}
	}

	// Make sure plugin is returning us something that obeys the PluginSpec.
	object, err := util.RuntimeToUnstructured(pluginObj)
	if err != nil {
		return nil, resourceStatusError{
			err: errors.Wrap(err, "plugin output cannot be converted from runtime.Object"),
		}
	}
	expectedGVK := pluginContainer.Plugin.Describe().GVK
	if object.GroupVersionKind() != expectedGVK {
		return nil, resourceStatusError{
			err: errors.Errorf("unexpected GVK from plugin (wanted %s, got %s)", expectedGVK, object.GroupVersionKind()),
		}
	}
	// We are in charge of naming.
	object.SetName(res.Spec.Plugin.ObjectName)

	return object, nil
}

func (st *resourceSyncTask) prepareDependencies(references []smith_v1.Reference) (map[smith_v1.ResourceName]plugin.Dependency, error) {
	dependencies := make(map[smith_v1.ResourceName]plugin.Dependency)
	for _, reference := range references {
		if _, ok := dependencies[reference.Resource]; ok {
			// References could refer to the same resource as a previous one.
			continue
		}
		unstructuredActual := st.processedResources[reference.Resource].actual.DeepCopy() // Pass a copy to the plugin to insulate from it
		gvk := unstructuredActual.GroupVersionKind()
		var actual runtime.Object
		if st.scheme.Recognizes(gvk) {
			// Convert to typed object if we recognize the type
			var err error
			actual, err = st.scheme.ConvertToVersion(unstructuredActual, gvk.GroupVersion())
			if err != nil {
				return nil, errors.WithStack(err)
			}
			actual.GetObjectKind().SetGroupVersionKind(gvk)
		} else {
			actual = unstructuredActual
		}
		dependency := plugin.Dependency{
			Actual: actual,
		}

		if obj, ok := actual.(*sc_v1b1.ServiceBinding); ok {
			if err := st.prepareServiceBindingDependency(&dependency, obj); err != nil {
				return nil, errors.Wrapf(err, "error processing ServiceBinding %q", obj.Name)
			}
		}
		dependencies[reference.Resource] = dependency
	}
	return dependencies, nil
}

func (st *resourceSyncTask) prepareServiceBindingDependency(dependency *plugin.Dependency, obj *sc_v1b1.ServiceBinding) error {
	secret, exists, err := st.store.Get(core_v1.SchemeGroupVersion.WithKind("Secret"), obj.Namespace, obj.Spec.SecretName)
	if err != nil {
		return errors.Wrap(err, "error finding output Secret")
	}
	if !exists {
		return errors.New("cannot find output Secret")
	}
	dependency.Outputs = append(dependency.Outputs, secret)

	serviceInstance, exists, err := st.store.Get(sc_v1b1.SchemeGroupVersion.WithKind("ServiceInstance"), obj.Namespace, obj.Spec.InstanceRef.Name)
	if err != nil {
		return errors.Wrapf(err, "error finding ServiceInstance %q", obj.Spec.InstanceRef.Name)
	}
	if !exists {
		return errors.Errorf("cannot find ServiceInstance %q", obj.Spec.InstanceRef.Name)
	}
	dependency.Auxiliary = append(dependency.Auxiliary, serviceInstance)
	return nil
}

// createOrUpdate creates or updates a resources.
func (st *resourceSyncTask) createOrUpdate(spec *unstructured.Unstructured, actual runtime.Object) (actualRet *unstructured.Unstructured, retriableRet bool, e error) {
	// Prepare client
	gvk := spec.GroupVersionKind()
	resClient, err := st.smartClient.ForGVK(gvk, st.bundle.Namespace)
	if err != nil {
		return nil, false, errors.Wrapf(err, "failed to get the client for %s", gvk)
	}
	switch actual {
	case nil:
		return st.createResource(resClient, spec)
	default:
		return st.updateResource(resClient, spec, actual)
	}
}

func (st *resourceSyncTask) createResource(resClient dynamic.ResourceInterface, spec *unstructured.Unstructured) (actualRet *unstructured.Unstructured, retriableError bool, e error) {
	spec, err := st.specChecker.BeforeCreate(st.logger, spec)
	if err != nil {
		return nil, false, errors.Wrap(err, "object specification pre-processing failed")
	}
	gvk := spec.GroupVersionKind()
	st.logger.Debug("Object not found, creating", ctrlLogz.ObjectGk(gvk.GroupKind()), ctrlLogz.Object(spec))
	response, err := resClient.Create(spec, meta_v1.CreateOptions{})
	if err == nil {
		st.logger.Info("Object created", ctrlLogz.ObjectGk(gvk.GroupKind()), ctrlLogz.Object(spec))
		return response, false, nil
	}
	if api_errors.IsAlreadyExists(err) {
		// We let the next processKey() iteration, triggered by someone else creating the resource, to finish the work.
		err = api_errors.NewConflict(schema.GroupResource{Group: gvk.Group, Resource: gvk.Kind}, spec.GetName(), err)
		return nil, false, errors.Wrap(err, "object found, but not in Store yet (will re-process)")
	}
	// Unexpected error, will retry
	apiStatusErr, ok := err.(api_errors.APIStatus)
	if ok {
		apiStatus := apiStatusErr.Status()
		return nil, true, errors.Wrapf(err, "unexpected APIStatus (code %v, reason %q) while creating resource", apiStatus.Code, apiStatus.Reason)
	}
	return nil, true, errors.WithStack(err)
}

// Mutates spec and actual.
func (st *resourceSyncTask) updateResource(resClient dynamic.ResourceInterface, spec *unstructured.Unstructured, actual runtime.Object) (actualRet *unstructured.Unstructured, retriableError bool, e error) {
	st.logger.Debug("Object found, checking spec", ctrlLogz.ObjectGk(spec.GroupVersionKind().GroupKind()), ctrlLogz.Object(spec))
	// Compare spec and existing resource
	updated, match, difference, err := st.specChecker.CompareActualVsSpec(st.logger, spec, actual)
	if err != nil {
		return nil, false, errors.Wrap(err, "specification check failed")
	}

	// Delete the DeletionTimestamp annotation if it is present
	annotations := updated.GetAnnotations()
	if _, ok := annotations[smith.DeletionTimestampAnnotation]; ok {
		delete(annotations, smith.DeletionTimestampAnnotation)
		updated.SetAnnotations(annotations)
		match = false
	}

	if match {
		st.logger.Debug("Object has correct spec", ctrlLogz.Object(spec))
		return updated, false, nil
	}
	st.logger.Sugar().Infof("Objects are different (`a` is specification and `b` is the actual object): %s", difference)

	// Update if different
	updated, err = resClient.Update(updated, meta_v1.UpdateOptions{})
	if err != nil {
		if api_errors.IsConflict(err) {
			// We let the next processKey() iteration, triggered by someone else updating the resource, finish the work.
			return nil, false, errors.Wrap(err, "object update resulted in conflict (will re-process)")
		}
		// Unexpected error, will retry
		apiStatusErr, ok := err.(api_errors.APIStatus)
		if ok {
			apiStatus := apiStatusErr.Status()
			return nil, true, errors.Wrapf(err, "unexpected APIStatus (code %v, reason %q) while creating resource", apiStatus.Code, apiStatus.Reason)
		}
		return nil, true, errors.WithStack(err)
	}
	st.logger.Info("Object updated", ctrlLogz.Object(spec))
	return updated, false, nil
}
