package types

import (
	"time"

	"github.com/atlassian/smith/pkg/readychecker"
	"github.com/atlassian/smith/pkg/util"

	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	"github.com/pkg/errors"
	apps_v1 "k8s.io/api/apps/v1"
	autoscaling_v2b1 "k8s.io/api/autoscaling/v2beta1"
	core_v1 "k8s.io/api/core/v1"
	ext_v1b1 "k8s.io/api/extensions/v1beta1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	hpaAbleToScaleCondition   = autoscaling_v2b1.HorizontalPodAutoscalerConditionType("AbleToScale")
	hpaScalingActiveCondition = autoscaling_v2b1.HorizontalPodAutoscalerConditionType("ScalingActive")
)

var (
	MainKnownTypes = map[schema.GroupKind]readychecker.IsObjectReady{
		{Group: core_v1.GroupName, Kind: "ConfigMap"}:                        alwaysReady,
		{Group: core_v1.GroupName, Kind: "Secret"}:                           alwaysReady,
		{Group: core_v1.GroupName, Kind: "Service"}:                          alwaysReady,
		{Group: core_v1.GroupName, Kind: "ServiceAccount"}:                   alwaysReady,
		{Group: apps_v1.GroupName, Kind: "Deployment"}:                       isDeploymentReady,
		{Group: ext_v1b1.GroupName, Kind: "Ingress"}:                         alwaysReady,
		{Group: autoscaling_v2b1.GroupName, Kind: "HorizontalPodAutoscaler"}: isHorizontalPodAutoscalerReady,
	}
	ServiceCatalogKnownTypes = map[schema.GroupKind]readychecker.IsObjectReady{
		{Group: sc_v1b1.GroupName, Kind: "ServiceBinding"}:  isScServiceBindingReady,
		{Group: sc_v1b1.GroupName, Kind: "ServiceInstance"}: isScServiceInstanceReady,
	}
	appsV1Scheme    = runtime.NewScheme()
	scV1B1Scheme    = runtime.NewScheme()
	autoscalingV2B1 = runtime.NewScheme()
)

func init() {
	err := apps_v1.SchemeBuilder.AddToScheme(appsV1Scheme)
	if err != nil {
		panic(err)
	}
	err = sc_v1b1.SchemeBuilder.AddToScheme(scV1B1Scheme)
	if err != nil {
		panic(err)
	}
	err = autoscaling_v2b1.SchemeBuilder.AddToScheme(autoscalingV2B1)
	if err != nil {
		panic(err)
	}
}

func alwaysReady(_ runtime.Object) (isReady, retriableError bool, e error) {
	return true, false, nil
}

// Works according to https://kubernetes.io/docs/user-guide/deployments/#the-status-of-a-deployment
// and k8s.io/kubernetes/pkg/client/unversioned/conditions.go:120 DeploymentHasDesiredReplicas()
func isDeploymentReady(obj runtime.Object) (isReady, retriableError bool, e error) {
	var deployment apps_v1.Deployment
	if err := util.ConvertType(appsV1Scheme, obj, &deployment); err != nil {
		return false, false, err
	}

	replicas := int32(1) // Default value if not specified
	if deployment.Spec.Replicas != nil {
		replicas = *deployment.Spec.Replicas
	}

	return deployment.Status.ObservedGeneration >= deployment.Generation &&
		deployment.Status.UpdatedReplicas == replicas, false, nil
}

func isHorizontalPodAutoscalerReady(obj runtime.Object) (isReady, retriableError bool, e error) {
	var hpa autoscaling_v2b1.HorizontalPodAutoscaler
	if err := util.ConvertType(autoscalingV2B1, obj, &hpa); err != nil {
		return false, false, err
	}

	// For the HPA to be ready, AbleToScale and ScalingActive conditions should exist and be true
	// If ScalingActive is false, it could still be coming up *or* it could have failed (e.g. because it can't access metrics)
	// Can't distinguish this based on the reason, so using a timeout
	foundAbleToScale := false
	foundScalingActive := false
	for _, cond := range hpa.Status.Conditions {
		switch cond.Type {
		case hpaAbleToScaleCondition:
			switch cond.Status {
			case core_v1.ConditionTrue:
				foundAbleToScale = true
			case core_v1.ConditionFalse:
				// AbleToScale should not be false if the HPA is working, this is a failure
				return false, false, errors.Errorf("%s: %s", cond.Reason, cond.Message)
			case core_v1.ConditionUnknown:
				// Assume it's still coming up
			}
		case hpaScalingActiveCondition:
			switch cond.Status {
			case core_v1.ConditionTrue:
				foundScalingActive = true
			case core_v1.ConditionFalse:
				// It's expected that it will take a little while to get metrics
				// If it's been stuck in this condition for >3min we assume it's failed
				now := meta_v1.Now()
				if cond.LastTransitionTime.Add(3 * time.Minute).Before(now.Time) {
					return false, false, errors.Errorf("%s: %s", cond.Reason, cond.Message)
				}
			case core_v1.ConditionUnknown:
				// Assume it's still coming up
			}
		}
	}

	return foundAbleToScale && foundScalingActive, false, nil
}

func isScServiceBindingReady(obj runtime.Object) (isReady, retriableError bool, e error) {
	var sic sc_v1b1.ServiceBinding
	if err := util.ConvertType(scV1B1Scheme, obj, &sic); err != nil {
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

func isScServiceInstanceReady(obj runtime.Object) (isReady, retriableError bool, e error) {
	var instance sc_v1b1.ServiceInstance
	if err := util.ConvertType(scV1B1Scheme, obj, &instance); err != nil {
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
