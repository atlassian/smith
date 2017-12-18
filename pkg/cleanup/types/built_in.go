package types

import (
	"errors"

	"github.com/atlassian/smith/pkg/cleanup"
	"github.com/atlassian/smith/pkg/util"

	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	apps_v1b2 "k8s.io/api/apps/v1beta2"
	core_v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	unstructured_conversion "k8s.io/apimachinery/pkg/conversion/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var MainKnownTypes = map[schema.GroupKind]cleanup.SpecCleanup{
	{Group: apps_v1b2.GroupName, Kind: "Deployment"}: deploymentCleanup,
	{Group: core_v1.GroupName, Kind: "Service"}:      serviceCleanup,
}

var ServiceCatalogKnownTypes = map[schema.GroupKind]cleanup.SpecCleanup{
	{Group: sc_v1b1.GroupName, Kind: "ServiceBinding"}:  scServiceBindingCleanup,
	{Group: sc_v1b1.GroupName, Kind: "ServiceInstance"}: scServiceInstanceCleanup,
}

func deploymentCleanup(spec, actual *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	var deployment apps_v1b2.Deployment
	if err := unstructured_conversion.DefaultConverter.FromUnstructured(spec.Object, &deployment); err != nil {
		return nil, err
	}

	// TODO: implement cleanup

	return spec, nil
}

func serviceCleanup(spec, actual *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	var serviceSpec core_v1.Service
	if err := unstructured_conversion.DefaultConverter.FromUnstructured(spec.Object, &serviceSpec); err != nil {
		return nil, err
	}
	var serviceActual core_v1.Service
	if err := unstructured_conversion.DefaultConverter.FromUnstructured(actual.Object, &serviceActual); err != nil {
		return nil, err
	}

	serviceSpec.Spec.ClusterIP = serviceActual.Spec.ClusterIP
	serviceSpec.Status = serviceActual.Status

	if len(serviceActual.Spec.Ports) == len(serviceSpec.Spec.Ports) {
		for i, port := range serviceSpec.Spec.Ports {
			if port.NodePort == 0 {
				actualPort := serviceActual.Spec.Ports[i]
				port.NodePort = actualPort.NodePort
				if port == actualPort { // NodePort field is the only difference, other fields are the same
					// Copy port from actual if port is not specified in spec
					serviceSpec.Spec.Ports[i].NodePort = actualPort.NodePort
				}
			}
		}
	}

	return util.RuntimeToUnstructured(&serviceSpec)
}

func scServiceBindingCleanup(spec, actual *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	var sbSpec sc_v1b1.ServiceBinding
	if err := unstructured_conversion.DefaultConverter.FromUnstructured(spec.Object, &sbSpec); err != nil {
		return nil, err
	}
	var sbActual sc_v1b1.ServiceBinding
	if err := unstructured_conversion.DefaultConverter.FromUnstructured(actual.Object, &sbActual); err != nil {
		return nil, err
	}

	sbSpec.Spec.ExternalID = sbActual.Spec.ExternalID
	sbSpec.Status = sbActual.Status

	return util.RuntimeToUnstructured(&sbSpec)
}

func scServiceInstanceCleanup(spec, actual *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	var instanceSpec sc_v1b1.ServiceInstance
	if err := unstructured_conversion.DefaultConverter.FromUnstructured(spec.Object, &instanceSpec); err != nil {
		return nil, err
	}
	var instanceActual sc_v1b1.ServiceInstance
	if err := unstructured_conversion.DefaultConverter.FromUnstructured(actual.Object, &instanceActual); err != nil {
		return nil, err
	}

	if instanceSpec.Spec.ClusterServiceClassName != "" &&
		instanceSpec.Spec.ClusterServiceClassName != instanceActual.Spec.ClusterServiceClassName {
		return nil, errors.New("clusterServiceClassName has changed when it should be immutable")
	}

	if instanceSpec.Spec.ClusterServicePlanName != "" &&
		instanceSpec.Spec.ClusterServicePlanName != instanceActual.Spec.ClusterServicePlanName {
		return nil, errors.New("clusterServicePlanName has changed when it should be immutable")
	}

	if instanceActual.Spec.ClusterServiceClassExternalName == instanceSpec.Spec.ClusterServiceClassExternalName {
		instanceSpec.Spec.ClusterServiceClassRef = instanceActual.Spec.ClusterServiceClassRef
		instanceSpec.Spec.ClusterServiceClassName = instanceActual.Spec.ClusterServiceClassName
	}

	if instanceActual.Spec.ClusterServicePlanExternalName == instanceSpec.Spec.ClusterServicePlanExternalName {
		instanceSpec.Spec.ClusterServicePlanRef = instanceActual.Spec.ClusterServicePlanRef
		instanceSpec.Spec.ClusterServicePlanName = instanceActual.Spec.ClusterServicePlanName
	}

	instanceSpec.ObjectMeta.Finalizers = instanceActual.ObjectMeta.Finalizers

	instanceSpec.Spec.ExternalID = instanceActual.Spec.ExternalID
	if instanceActual.Spec.UserInfo != nil {
		instanceSpec.Spec.UserInfo = instanceActual.Spec.UserInfo.DeepCopy()
	}

	return util.RuntimeToUnstructured(&instanceSpec)
}
