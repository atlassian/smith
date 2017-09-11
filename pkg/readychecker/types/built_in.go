package types

import (
	"github.com/atlassian/smith/pkg/readychecker"

	sc_v1a1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1alpha1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	unstructured_conversion "k8s.io/apimachinery/pkg/conversion/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	api_v1 "k8s.io/client-go/pkg/api/v1"
	apps_v1b1 "k8s.io/client-go/pkg/apis/apps/v1beta1"
	ext_v1b1 "k8s.io/client-go/pkg/apis/extensions/v1beta1"
	settings_v1a1 "k8s.io/client-go/pkg/apis/settings/v1alpha1"
)

var MainKnownTypes = map[schema.GroupKind]readychecker.IsObjectReady{
	{Group: api_v1.GroupName, Kind: "ConfigMap"}:        alwaysReady,
	{Group: api_v1.GroupName, Kind: "Secret"}:           alwaysReady,
	{Group: api_v1.GroupName, Kind: "Service"}:          alwaysReady,
	{Group: apps_v1b1.GroupName, Kind: "Deployment"}:    isDeploymentReady,
	{Group: settings_v1a1.GroupName, Kind: "PodPreset"}: alwaysReady,
	{Group: ext_v1b1.GroupName, Kind: "Ingress"}:        alwaysReady,
}

var ServiceCatalogKnownTypes = map[schema.GroupKind]readychecker.IsObjectReady{
	{Group: sc_v1a1.GroupName, Kind: "ServiceInstanceCredential"}: isScServiceInstanceCredentialReady,
	{Group: sc_v1a1.GroupName, Kind: "ServiceInstance"}:           isScServiceInstanceReady,
}

func alwaysReady(_ *unstructured.Unstructured) (isReady, retriableError bool, e error) {
	return true, false, nil
}

// Works according to https://kubernetes.io/docs/user-guide/deployments/#the-status-of-a-deployment
// and k8s.io/kubernetes/pkg/client/unversioned/conditions.go:120 DeploymentHasDesiredReplicas()
func isDeploymentReady(obj *unstructured.Unstructured) (isReady, retriableError bool, e error) {
	var deployment apps_v1b1.Deployment
	if err := unstructured_conversion.DefaultConverter.FromUnstructured(obj.Object, &deployment); err != nil {
		return false, false, err
	}

	replicas := int32(1) // Default value if not specified
	if deployment.Spec.Replicas != nil {
		replicas = *deployment.Spec.Replicas
	}

	return deployment.Status.ObservedGeneration >= deployment.Generation &&
		deployment.Status.UpdatedReplicas == replicas, false, nil
}

func isScServiceInstanceCredentialReady(obj *unstructured.Unstructured) (isReady, retriableError bool, e error) {
	var sic sc_v1a1.ServiceInstanceCredential
	if err := unstructured_conversion.DefaultConverter.FromUnstructured(obj.Object, &sic); err != nil {
		return false, false, err
	}
	readyCond := getServiceInstanceCredentialCondition(&sic, sc_v1a1.ServiceInstanceCredentialConditionReady)
	return readyCond != nil && readyCond.Status == sc_v1a1.ConditionTrue, false, nil
}

func isScServiceInstanceReady(obj *unstructured.Unstructured) (isReady, retriableError bool, e error) {
	var instance sc_v1a1.ServiceInstance
	if err := unstructured_conversion.DefaultConverter.FromUnstructured(obj.Object, &instance); err != nil {
		return false, false, err
	}
	readyCond := getServiceInstanceCondition(&instance, sc_v1a1.ServiceInstanceConditionReady)
	return readyCond != nil && readyCond.Status == sc_v1a1.ConditionTrue, false, nil
}

func getServiceInstanceCondition(instance *sc_v1a1.ServiceInstance, conditionType sc_v1a1.ServiceInstanceConditionType) *sc_v1a1.ServiceInstanceCondition {
	for _, condition := range instance.Status.Conditions {
		if condition.Type == conditionType {
			return &condition
		}
	}
	return nil
}

func getServiceInstanceCredentialCondition(instance *sc_v1a1.ServiceInstanceCredential, conditionType sc_v1a1.ServiceInstanceCredentialConditionType) *sc_v1a1.ServiceInstanceCredentialCondition {
	for _, condition := range instance.Status.Conditions {
		if condition.Type == conditionType {
			return &condition
		}
	}
	return nil
}
