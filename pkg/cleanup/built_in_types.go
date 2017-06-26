package cleanup

import (
	sc_v1a1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1alpha1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	unstructured_conversion "k8s.io/apimachinery/pkg/conversion/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	api_v1 "k8s.io/client-go/pkg/api/v1"
	apps_v1b1 "k8s.io/client-go/pkg/apis/apps/v1beta1"
)

var converter = unstructured_conversion.NewConverter(false)

var MainKnownTypes = map[schema.GroupKind]SpecCleanup{
	{Group: apps_v1b1.GroupName, Kind: "Deployment"}: deploymentCleanup,
	{Group: api_v1.GroupName, Kind: "Service"}:       serviceCleanup,
}

var ServiceCatalogKnownTypes = map[schema.GroupKind]SpecCleanup{
	{Group: sc_v1a1.GroupName, Kind: "Binding"}:  scBindingCleanup,
	{Group: sc_v1a1.GroupName, Kind: "Instance"}: scInstanceCleanup,
}

func deploymentCleanup(obj *unstructured.Unstructured, actual *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	var deployment apps_v1b1.Deployment
	if err := converter.FromUnstructured(obj.Object, &deployment); err != nil {
		return nil, err
	}

	// TODO: implement cleanup

	return obj, nil
}

func serviceCleanup(spec *unstructured.Unstructured, actual *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	var serviceSpec api_v1.Service
	if err := converter.FromUnstructured(spec.Object, &serviceSpec); err != nil {
		return nil, err
	}
	var serviceActual api_v1.Service
	if err := converter.FromUnstructured(actual.Object, &serviceActual); err != nil {
		return nil, err
	}

	serviceSpec.Spec.ClusterIP = serviceActual.Spec.ClusterIP
	serviceSpec.Status = serviceActual.Status

	updatedObj := &unstructured.Unstructured{
		Object: make(map[string]interface{}),
	}
	converter.ToUnstructured(&serviceSpec, &updatedObj.Object)
	return updatedObj, nil
}

func scBindingCleanup(spec *unstructured.Unstructured, actual *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	var bindingSpec sc_v1a1.Binding
	if err := converter.FromUnstructured(spec.Object, &bindingSpec); err != nil {
		return nil, err
	}
	var bindingActual sc_v1a1.Binding
	if err := converter.FromUnstructured(actual.Object, &bindingActual); err != nil {
		return nil, err
	}

	bindingSpec.Spec.ExternalID = bindingActual.Spec.ExternalID
	bindingSpec.Status = bindingActual.Status

	updatedObj := &unstructured.Unstructured{
		Object: make(map[string]interface{}),
	}
	converter.ToUnstructured(&bindingSpec, &updatedObj.Object)
	return updatedObj, nil
}

func scInstanceCleanup(spec *unstructured.Unstructured, actual *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	var instanceSpec sc_v1a1.Instance
	if err := converter.FromUnstructured(spec.Object, &instanceSpec); err != nil {
		return nil, err
	}
	var instanceActual sc_v1a1.Instance
	if err := converter.FromUnstructured(actual.Object, &instanceActual); err != nil {
		return nil, err
	}

	instanceSpec.Spec.ExternalID = instanceActual.Spec.ExternalID

	updatedObj := &unstructured.Unstructured{
		Object: make(map[string]interface{}),
	}
	err := converter.ToUnstructured(&instanceSpec, &updatedObj.Object)
	if err != nil {
		return nil, err
	}
	return updatedObj, nil
}
