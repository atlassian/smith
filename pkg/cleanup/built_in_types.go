package cleanup

import (
	"fmt"

	sc_v1a1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1alpha1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	unstructured_conversion "k8s.io/apimachinery/pkg/conversion/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	apps_v1b1 "k8s.io/client-go/pkg/apis/apps/v1beta1"
	ext_v1b1 "k8s.io/client-go/pkg/apis/extensions/v1beta1"
)

var converter = unstructured_conversion.NewConverter(false)

var MainKnownTypes = map[schema.GroupKind]Cleanup{
	{Group: apps_v1b1.GroupName, Kind: "Deployment"}: deploymentAppsCleanup,
	{Group: ext_v1b1.GroupName, Kind: "Deployment"}:  deploymentExtCleanup,
}

var ServiceCatalogKnownTypes = map[schema.GroupKind]Cleanup{
	{Group: sc_v1a1.GroupName, Kind: "Binding"}:  scBindingCleanup,
	{Group: sc_v1a1.GroupName, Kind: "Instance"}: scInstanceCleanup,
}

func deploymentExtCleanup(obj *unstructured.Unstructured) (err error) {
	var deployment ext_v1b1.Deployment
	if err := converter.FromUnstructured(obj.Object, &deployment); err != nil {
		return err
	}

	// TODO: implement cleanup
	fmt.Print("deploymentExtCleanup\n")

	return nil
}

func deploymentAppsCleanup(obj *unstructured.Unstructured) (err error) {
	var deployment apps_v1b1.Deployment
	if err := converter.FromUnstructured(obj.Object, &deployment); err != nil {
		return err
	}

	// TODO: implement cleanup
	fmt.Print("deploymentAppsCleanup\n")

	return nil
}

func scBindingCleanup(obj *unstructured.Unstructured) (err error) {
	var binding sc_v1a1.Binding
	if err := converter.FromUnstructured(obj.Object, &binding); err != nil {
		return err
	}

	fmt.Printf("scBindingCleanup: binding.Spec.ExternalID=%s\n", binding.Spec.ExternalID)
	binding.Spec.ExternalID = ""

	return nil
}

func scInstanceCleanup(obj *unstructured.Unstructured) (err error) {
	var instance sc_v1a1.Instance
	if err := converter.FromUnstructured(obj.Object, &instance); err != nil {
		return err
	}

	fmt.Printf("scBindingCleanup: binding.Spec.ExternalID=%s\n", instance.Spec.ExternalID)
	instance.Spec.ExternalID = ""

	return nil
}
