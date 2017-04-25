package readychecker

import (
	sc_v0 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog"
	scv1alpha1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1alpha1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	unstructured_conversion "k8s.io/apimachinery/pkg/conversion/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	api_v0 "k8s.io/client-go/pkg/api"
	apps_v0 "k8s.io/client-go/pkg/apis/apps"
	appsv1beta1 "k8s.io/client-go/pkg/apis/apps/v1beta1"
	extensions_v0 "k8s.io/client-go/pkg/apis/extensions"
	extensions "k8s.io/client-go/pkg/apis/extensions/v1beta1"
	settings_v0 "k8s.io/client-go/pkg/apis/settings"
)

var converter = unstructured_conversion.NewConverter(false)

var MainKnownTypes = map[schema.GroupKind]IsObjectReady{
	{Group: api_v0.GroupName, Kind: "ConfigMap"}:         alwaysReady,
	{Group: api_v0.GroupName, Kind: "Secret"}:            alwaysReady,
	{Group: api_v0.GroupName, Kind: "Service"}:           alwaysReady,
	{Group: apps_v0.GroupName, Kind: "Deployment"}:       isDeploymentAppsReady,
	{Group: settings_v0.GroupName, Kind: "PodPreset"}:    alwaysReady,
	{Group: extensions_v0.GroupName, Kind: "Ingress"}:    alwaysReady,
	{Group: extensions_v0.GroupName, Kind: "Deployment"}: isDeploymentExtReady,
}

var ServiceCatalogKnownTypes = map[schema.GroupKind]IsObjectReady{
	{Group: sc_v0.GroupName, Kind: "Binding"}:  isScBindingReady,
	{Group: sc_v0.GroupName, Kind: "Instance"}: isScInstanceReady,
}

func alwaysReady(_ *unstructured.Unstructured) (isReady, retriableError bool, e error) {
	return true, false, nil
}

// Works according to https://kubernetes.io/docs/user-guide/deployments/#the-status-of-a-deployment
// and k8s.io/kubernetes/pkg/client/unversioned/conditions.go:120 DeploymentHasDesiredReplicas()
func isDeploymentExtReady(obj *unstructured.Unstructured) (isReady, retriableError bool, e error) {
	var deployment extensions.Deployment
	if err := converter.FromUnstructured(obj.Object, &deployment); err != nil {
		return false, false, err
	}

	replicas := int32(1) // Default value if not specified
	if deployment.Spec.Replicas != nil {
		replicas = *deployment.Spec.Replicas
	}

	return deployment.Status.ObservedGeneration >= deployment.Generation &&
		deployment.Status.UpdatedReplicas == replicas, false, nil
}

// Works according to https://kubernetes.io/docs/user-guide/deployments/#the-status-of-a-deployment
// and k8s.io/kubernetes/pkg/client/unversioned/conditions.go:120 DeploymentHasDesiredReplicas()
func isDeploymentAppsReady(obj *unstructured.Unstructured) (isReady, retriableError bool, e error) {
	var deployment appsv1beta1.Deployment
	if err := converter.FromUnstructured(obj.Object, &deployment); err != nil {
		return false, false, err
	}

	replicas := int32(1) // Default value if not specified
	if deployment.Spec.Replicas != nil {
		replicas = *deployment.Spec.Replicas
	}

	return deployment.Status.ObservedGeneration >= deployment.Generation &&
		deployment.Status.UpdatedReplicas == replicas, false, nil
}

func isScBindingReady(obj *unstructured.Unstructured) (isReady, retriableError bool, e error) {
	var binding scv1alpha1.Binding
	if err := converter.FromUnstructured(obj.Object, &binding); err != nil {
		return false, false, err
	}
	readyCond := getBindingCondition(&binding, scv1alpha1.BindingConditionReady)
	return readyCond != nil && readyCond.Status == scv1alpha1.ConditionTrue, false, nil
}

func isScInstanceReady(obj *unstructured.Unstructured) (isReady, retriableError bool, e error) {
	var instance scv1alpha1.Instance
	if err := converter.FromUnstructured(obj.Object, &instance); err != nil {
		return false, false, err
	}
	readyCond := getInstanceCondition(&instance, scv1alpha1.InstanceConditionReady)
	return readyCond != nil && readyCond.Status == scv1alpha1.ConditionTrue, false, nil
}

func getInstanceCondition(instance *scv1alpha1.Instance, conditionType scv1alpha1.InstanceConditionType) *scv1alpha1.InstanceCondition {
	for _, condition := range instance.Status.Conditions {
		if condition.Type == conditionType {
			return &condition
		}
	}
	return nil
}

func getBindingCondition(instance *scv1alpha1.Binding, conditionType scv1alpha1.BindingConditionType) *scv1alpha1.BindingCondition {
	for _, condition := range instance.Status.Conditions {
		if condition.Type == conditionType {
			return &condition
		}
	}
	return nil
}
