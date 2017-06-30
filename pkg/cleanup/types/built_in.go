package types

import (
	"github.com/atlassian/smith/pkg/cleanup"

	sc_v1a1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1alpha1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	unstructured_conversion "k8s.io/apimachinery/pkg/conversion/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	api_v1 "k8s.io/client-go/pkg/api/v1"
	apps_v1b1 "k8s.io/client-go/pkg/apis/apps/v1beta1"
)

var MainKnownTypes = map[schema.GroupKind]cleanup.SpecCleanup{
	{Group: apps_v1b1.GroupName, Kind: "Deployment"}: deploymentCleanup,
	{Group: api_v1.GroupName, Kind: "Service"}:       serviceCleanup,
}

var ServiceCatalogKnownTypes = map[schema.GroupKind]cleanup.SpecCleanup{
	{Group: sc_v1a1.GroupName, Kind: "Binding"}:  scBindingCleanup,
	{Group: sc_v1a1.GroupName, Kind: "Instance"}: scInstanceCleanup,
}

func deploymentCleanup(spec, actual *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	var deployment apps_v1b1.Deployment
	if err := unstructured_conversion.DefaultConverter.FromUnstructured(spec.Object, &deployment); err != nil {
		return nil, err
	}

	// TODO: implement cleanup

	return spec, nil
}

func serviceCleanup(spec, actual *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	var serviceSpec api_v1.Service
	if err := unstructured_conversion.DefaultConverter.FromUnstructured(spec.Object, &serviceSpec); err != nil {
		return nil, err
	}
	var serviceActual api_v1.Service
	if err := unstructured_conversion.DefaultConverter.FromUnstructured(actual.Object, &serviceActual); err != nil {
		return nil, err
	}

	serviceSpec.Spec.ClusterIP = serviceActual.Spec.ClusterIP
	serviceSpec.Status = serviceActual.Status

	updatedObj := &unstructured.Unstructured{
		Object: make(map[string]interface{}),
	}
	if err := unstructured_conversion.DefaultConverter.ToUnstructured(&serviceSpec, &updatedObj.Object); err != nil {
		return nil, err
	}
	return updatedObj, nil
}

func scBindingCleanup(spec, actual *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	var bindingSpec sc_v1a1.Binding
	if err := unstructured_conversion.DefaultConverter.FromUnstructured(spec.Object, &bindingSpec); err != nil {
		return nil, err
	}
	var bindingActual sc_v1a1.Binding
	if err := unstructured_conversion.DefaultConverter.FromUnstructured(actual.Object, &bindingActual); err != nil {
		return nil, err
	}

	bindingSpec.Spec.ExternalID = bindingActual.Spec.ExternalID
	bindingSpec.Status = bindingActual.Status

	updatedObj := &unstructured.Unstructured{
		Object: make(map[string]interface{}),
	}
	if err := unstructured_conversion.DefaultConverter.ToUnstructured(&bindingSpec, &updatedObj.Object); err != nil {
		return nil, err
	}
	return updatedObj, nil
}

func scInstanceCleanup(spec, actual *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	var instanceSpec sc_v1a1.Instance
	if err := unstructured_conversion.DefaultConverter.FromUnstructured(spec.Object, &instanceSpec); err != nil {
		return nil, err
	}
	var instanceActual sc_v1a1.Instance
	if err := unstructured_conversion.DefaultConverter.FromUnstructured(actual.Object, &instanceActual); err != nil {
		return nil, err
	}

	instanceSpec.Spec.ExternalID = instanceActual.Spec.ExternalID

	updatedObj := &unstructured.Unstructured{
		Object: make(map[string]interface{}),
	}
	if err := unstructured_conversion.DefaultConverter.ToUnstructured(&instanceSpec, &updatedObj.Object); err != nil {
		return nil, err
	}
	return updatedObj, nil
}
