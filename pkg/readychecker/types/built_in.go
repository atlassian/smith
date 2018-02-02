package types

import (
	"github.com/atlassian/smith/pkg/readychecker"
	"github.com/atlassian/smith/pkg/util"

	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	"github.com/pkg/errors"
	apps_v1b2 "k8s.io/api/apps/v1beta2"
	core_v1 "k8s.io/api/core/v1"
	ext_v1b1 "k8s.io/api/extensions/v1beta1"
	settings_v1a1 "k8s.io/api/settings/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var MainKnownTypes = map[schema.GroupKind]readychecker.IsObjectReady{
	{Group: core_v1.GroupName, Kind: "ConfigMap"}:       alwaysReady,
	{Group: core_v1.GroupName, Kind: "Secret"}:          alwaysReady,
	{Group: core_v1.GroupName, Kind: "Service"}:         alwaysReady,
	{Group: apps_v1b2.GroupName, Kind: "Deployment"}:    isDeploymentReady,
	{Group: settings_v1a1.GroupName, Kind: "PodPreset"}: alwaysReady,
	{Group: ext_v1b1.GroupName, Kind: "Ingress"}:        alwaysReady,
}

var ServiceCatalogKnownTypes = map[schema.GroupKind]readychecker.IsObjectReady{
	{Group: sc_v1b1.GroupName, Kind: "ServiceBinding"}:  isScServiceBindingReady,
	{Group: sc_v1b1.GroupName, Kind: "ServiceInstance"}: isScServiceInstanceReady,
}

func alwaysReady(_ *runtime.Scheme, _ runtime.Object) (isReady, retriableError bool, e error) {
	return true, false, nil
}

// Works according to https://kubernetes.io/docs/user-guide/deployments/#the-status-of-a-deployment
// and k8s.io/kubernetes/pkg/client/unversioned/conditions.go:120 DeploymentHasDesiredReplicas()
func isDeploymentReady(scheme *runtime.Scheme, obj runtime.Object) (isReady, retriableError bool, e error) {
	var deployment apps_v1b2.Deployment
	if err := util.ConvertType(scheme, obj, &deployment); err != nil {
		return false, false, err
	}

	replicas := int32(1) // Default value if not specified
	if deployment.Spec.Replicas != nil {
		replicas = *deployment.Spec.Replicas
	}

	return deployment.Status.ObservedGeneration >= deployment.Generation &&
		deployment.Status.UpdatedReplicas == replicas, false, nil
}

func isScServiceBindingReady(scheme *runtime.Scheme, obj runtime.Object) (isReady, retriableError bool, e error) {
	var sic sc_v1b1.ServiceBinding
	if err := util.ConvertType(scheme, obj, &sic); err != nil {
		return false, false, err
	}
	readyCond := getServiceBindingCondition(&sic, sc_v1b1.ServiceBindingConditionReady)
	if readyCond != nil && readyCond.Status == sc_v1b1.ConditionTrue {
		return true, false, nil
	}
	failedCond := getServiceBindingCondition(&sic, sc_v1b1.ServiceBindingConditionFailed)
	if failedCond != nil && failedCond.Status == sc_v1b1.ConditionTrue {
		return false, false, errors.Errorf("%s: %s", failedCond.Reason, failedCond.Message)
	}
	// TODO support "unknown" and "in progress"
	return false, false, nil

}

func isScServiceInstanceReady(scheme *runtime.Scheme, obj runtime.Object) (isReady, retriableError bool, e error) {
	var instance sc_v1b1.ServiceInstance
	if err := util.ConvertType(scheme, obj, &instance); err != nil {
		return false, false, err
	}
	readyCond := getServiceInstanceCondition(&instance, sc_v1b1.ServiceInstanceConditionReady)
	if readyCond != nil && readyCond.Status == sc_v1b1.ConditionTrue {
		return true, false, nil
	}
	failedCond := getServiceInstanceCondition(&instance, sc_v1b1.ServiceInstanceConditionFailed)
	if failedCond != nil && failedCond.Status == sc_v1b1.ConditionTrue {
		return false, false, errors.Errorf("%s: %s", failedCond.Reason, failedCond.Message)
	}
	// TODO support "unknown" and "in progress"
	return false, false, nil
}

func getServiceInstanceCondition(instance *sc_v1b1.ServiceInstance, conditionType sc_v1b1.ServiceInstanceConditionType) *sc_v1b1.ServiceInstanceCondition {
	for _, condition := range instance.Status.Conditions {
		if condition.Type == conditionType {
			return &condition
		}
	}
	return nil
}

func getServiceBindingCondition(instance *sc_v1b1.ServiceBinding, conditionType sc_v1b1.ServiceBindingConditionType) *sc_v1b1.ServiceBindingCondition {
	for _, condition := range instance.Status.Conditions {
		if condition.Type == conditionType {
			return &condition
		}
	}
	return nil
}
