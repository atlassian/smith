package cleanup

import (
	"fmt"

	sc_v1a1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1alpha1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	unstructured_conversion "k8s.io/apimachinery/pkg/conversion/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	api_v1 "k8s.io/client-go/pkg/api/v1"
	apps_v1b1 "k8s.io/client-go/pkg/apis/apps/v1beta1"
)

var converter = unstructured_conversion.NewConverter(false)

var MainKnownTypes = map[schema.GroupKind]Cleanup{
	{Group: apps_v1b1.GroupName, Kind: "Deployment"}: deploymentCleanup,
	{Group: api_v1.GroupName, Kind: "Service"}:       serviceCleanup,
}

var ServiceCatalogKnownTypes = map[schema.GroupKind]Cleanup{
	{Group: sc_v1a1.GroupName, Kind: "Binding"}:  scBindingCleanup,
	{Group: sc_v1a1.GroupName, Kind: "Instance"}: scInstanceCleanup,
}

func deploymentCleanup(obj *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	var deployment apps_v1b1.Deployment
	if err := converter.FromUnstructured(obj.Object, &deployment); err != nil {
		return nil, err
	}

	// TODO: implement cleanup
	fmt.Print("deploymentCleanup\n")

	return obj, nil
}

func serviceCleanup(obj *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	var service api_v1.Service
	if err := converter.FromUnstructured(obj.Object, &service); err != nil {
		return nil, err
	}

	// TODO: implement cleanup
	fmt.Print("serviceCleanup\n")
	service.Spec.ClusterIP = "test"

	updatedObj := &unstructured.Unstructured{
		Object: make(map[string]interface{}),
	}
	converter.ToUnstructured(&service, &updatedObj.Object)
	return updatedObj, nil
}

func scBindingCleanup(obj *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	var binding sc_v1a1.Binding
	if err := converter.FromUnstructured(obj.Object, &binding); err != nil {
		return nil, err
	}

	fmt.Printf("scBindingCleanup: binding.Spec.ExternalID=%s\n", binding.Spec.ExternalID)
	binding.Spec.ExternalID = ""

	updatedObj := &unstructured.Unstructured{
		Object: make(map[string]interface{}),
	}
	converter.ToUnstructured(&binding, &updatedObj.Object)
	return updatedObj, nil
}

func scInstanceCleanup(obj *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	var instance sc_v1a1.Instance
	if err := converter.FromUnstructured(obj.Object, &instance); err != nil {
		return nil, err
	}

	fmt.Printf("scInstanceCleanup: binding.Spec.ExternalID=%s\n", instance.Spec.ExternalID)
	//instance.ObjectMeta.SetFinalizers(nil)
	instance.Spec.ExternalID = "test"

	updatedObj := &unstructured.Unstructured{
		Object: make(map[string]interface{}),
	}
	err := converter.ToUnstructured(&instance, &updatedObj.Object)
	if err != nil {
		return nil, err
	}
	return updatedObj, nil
}
