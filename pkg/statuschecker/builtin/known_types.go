package types

import (
	"fmt"
	"strings"
	"time"

	"github.com/atlassian/smith/pkg/statuschecker"
	"github.com/atlassian/smith/pkg/util"
	sc_v1b1 "github.com/kubernetes-sigs/service-catalog/pkg/apis/servicecatalog/v1beta1"
	"github.com/pkg/errors"
	apps_v1 "k8s.io/api/apps/v1"
	autoscaling_v2b1 "k8s.io/api/autoscaling/v2beta1"
	core_v1 "k8s.io/api/core/v1"
	net_v1b1 "k8s.io/api/networking/v1beta1"
	policy_v1 "k8s.io/api/policy/v1beta1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
)

const (
	// Timeout for how long an HPA with a "False" ScalingActive condition should be considered "In Progress"
	// before being considered a failure
	hpaScalingActiveTimeout = 5 * time.Minute
	timedOutReason          = "ProgressDeadlineExceeded"
)

var (
	MainKnownTypes = map[schema.GroupKind]statuschecker.ObjectStatusChecker{
		{Group: core_v1.GroupName, Kind: "ConfigMap"}:             alwaysReady,
		{Group: core_v1.GroupName, Kind: "Secret"}:                alwaysReady,
		{Group: core_v1.GroupName, Kind: "Service"}:               alwaysReady,
		{Group: core_v1.GroupName, Kind: "ServiceAccount"}:        alwaysReady,
		{Group: apps_v1.GroupName, Kind: "Deployment"}:            isDeploymentReady,
		{Group: net_v1b1.GroupName, Kind: "Ingress"}:              alwaysReady,
		{Group: policy_v1.GroupName, Kind: "PodDisruptionBudget"}: alwaysReady,

		{Group: autoscaling_v2b1.GroupName, Kind: "HorizontalPodAutoscaler"}: isHorizontalPodAutoscalerReady,
	}
	ServiceCatalogKnownTypes = map[schema.GroupKind]statuschecker.ObjectStatusChecker{
		{Group: sc_v1b1.GroupName, Kind: "ServiceBinding"}:  isScServiceBindingReady,
		{Group: sc_v1b1.GroupName, Kind: "ServiceInstance"}: isScServiceInstanceReady,
	}
	appsV1Scheme          = runtime.NewScheme()
	scV1B1Scheme          = runtime.NewScheme()
	autoscalingV2B1Scheme = runtime.NewScheme()
)

var scNonErrorReasons = sets.NewString(
	"Provisioning",
	"UpdatingInstance",
	"Deprovisioning",
	"ProvisionRequestInFlight",
	"UpdateInstanceRequestInFlight",
	"DeprovisionRequestInFlight",
	"StartingInstanceOrphanMitigation",
)

func init() {
	utilruntime.Must(apps_v1.SchemeBuilder.AddToScheme(appsV1Scheme))
	utilruntime.Must(sc_v1b1.SchemeBuilder.AddToScheme(scV1B1Scheme))
	utilruntime.Must(autoscaling_v2b1.SchemeBuilder.AddToScheme(autoscalingV2B1Scheme))
}

func alwaysReady(_ runtime.Object) statuschecker.ObjectStatusResult {
	return statuschecker.ObjectStatusReady{}
}

func getDeploymentCondition(deployment *apps_v1.Deployment, condType apps_v1.DeploymentConditionType) *apps_v1.DeploymentCondition {
	deploymentStatus := deployment.Status
	for i := range deploymentStatus.Conditions {
		c := deploymentStatus.Conditions[i]
		if c.Type == condType {
			return &c
		}
	}
	return nil
}

// Works according to https://kubernetes.io/docs/user-guide/deployments/#the-status-of-a-deployment
func isDeploymentReady(obj runtime.Object) statuschecker.ObjectStatusResult {
	var deployment apps_v1.Deployment
	if err := util.ConvertType(appsV1Scheme, obj, &deployment); err != nil {
		return statuschecker.ObjectStatusError{
			Error: err,
		}
	}

	replicas := deployment.Spec.Replicas

	generation := deployment.Generation
	observedGeneration := deployment.Status.ObservedGeneration
	updatedReplicas := deployment.Status.UpdatedReplicas
	availableReplicas := deployment.Status.AvailableReplicas

	if generation <= observedGeneration {
		progressingCond := getDeploymentCondition(&deployment, apps_v1.DeploymentProgressing)
		if progressingCond != nil && progressingCond.Reason == timedOutReason {
			return statuschecker.ObjectStatusError{
				ExternalError:  true,
				RetriableError: false,
				Error:          errors.Errorf("deployment exceeded its progress deadline"),
			}
		}
		if replicas != nil && updatedReplicas < *replicas {
			return statuschecker.ObjectStatusInProgress{
				Message: fmt.Sprintf("Number of replicas converging. Requested=%d, Updated=%d", *replicas, updatedReplicas),
			}
		}

		if deployment.Status.Replicas > updatedReplicas {
			return statuschecker.ObjectStatusInProgress{
				Message: fmt.Sprintf("Number of replicas converging. Replicas=%d, Updated=%d", deployment.Status.Replicas, updatedReplicas),
			}
		}

		if availableReplicas < updatedReplicas {
			return statuschecker.ObjectStatusInProgress{
				Message: fmt.Sprintf("Number of replicas converging. Available=%d, Updated=%d", availableReplicas, updatedReplicas),
			}
		}

		return statuschecker.ObjectStatusReady{}
	}

	return statuschecker.ObjectStatusInProgress{
		Message: "Deployment in progress",
	}
}

func isHorizontalPodAutoscalerReady(obj runtime.Object) statuschecker.ObjectStatusResult {
	var hpa autoscaling_v2b1.HorizontalPodAutoscaler
	if err := util.ConvertType(autoscalingV2B1Scheme, obj, &hpa); err != nil {
		return statuschecker.ObjectStatusError{
			Error: err,
		}
	}

	// For the HPA to be ready, AbleToScale and ScalingActive conditions should exist and be true
	// If ScalingActive is false, it could still be coming up *or* it could have failed (e.g. because it can't access metrics)
	// Can't distinguish this based on the reason, so using a timeout
	foundAbleToScale := false
	foundScalingActive := false
	for _, cond := range hpa.Status.Conditions {
		switch cond.Type {
		case autoscaling_v2b1.AbleToScale:
			switch cond.Status {
			case core_v1.ConditionTrue:
				foundAbleToScale = true
			case core_v1.ConditionFalse:
				// AbleToScale should not be false if the HPA is working, this is a (retriable) failure
				return statuschecker.ObjectStatusError{
					Error:          errors.Errorf("%s: %s", cond.Reason, cond.Message),
					ExternalError:  true,
					RetriableError: true,
				}
			case core_v1.ConditionUnknown:
				// Assume it's still coming up
			}
		case autoscaling_v2b1.ScalingActive:
			switch cond.Status {
			case core_v1.ConditionTrue:
				foundScalingActive = true
			case core_v1.ConditionFalse:
				// It's expected that it will take a little while to get metrics
				// Assume it has failed after a timeout
				now := meta_v1.Now()
				if cond.LastTransitionTime.Add(hpaScalingActiveTimeout).Before(now.Time) {
					return statuschecker.ObjectStatusError{
						Error:          errors.Errorf("%s: %s", cond.Reason, cond.Message),
						ExternalError:  true,
						RetriableError: true,
					}
				}
			case core_v1.ConditionUnknown:
				// Assume it's still coming up
			}
		}
	}

	if foundAbleToScale && foundScalingActive {
		return statuschecker.ObjectStatusReady{}
	}
	return statuschecker.ObjectStatusInProgress{
		Message: fmt.Sprintf("ableToScale: %v, scalingActive: %v", foundAbleToScale, foundScalingActive),
	}
}

func isScServiceBindingReady(obj runtime.Object) statuschecker.ObjectStatusResult {
	var sic sc_v1b1.ServiceBinding
	if err := util.ConvertType(scV1B1Scheme, obj, &sic); err != nil {
		return statuschecker.ObjectStatusError{
			Error: err,
		}
	}
	readyCond := getServiceBindingCondition(&sic, sc_v1b1.ServiceBindingConditionReady)
	if readyCond != nil {
		switch readyCond.Status {
		case sc_v1b1.ConditionFalse:
			return statuschecker.ObjectStatusInProgress{
				Message: fmt.Sprintf("%v: %v", readyCond.Reason, readyCond.Message),
			}
		case sc_v1b1.ConditionTrue:
			var msg []string
			if len(readyCond.Reason) != 0 {
				msg = append(msg, readyCond.Reason)
			}
			if len(readyCond.Message) != 0 {
				msg = append(msg, readyCond.Message)
			}
			return statuschecker.ObjectStatusReady{
				Message: strings.Join(msg, ": "),
			}
		default:
			return statuschecker.ObjectStatusUnknown{
				Details: fmt.Sprintf("status is %q", readyCond.Status),
			}
		}
	}
	failedCond := getServiceBindingCondition(&sic, sc_v1b1.ServiceBindingConditionFailed)
	if failedCond != nil && failedCond.Status == sc_v1b1.ConditionTrue {
		return statuschecker.ObjectStatusError{
			Error:         errors.Errorf("%s: %s", failedCond.Reason, failedCond.Message),
			ExternalError: true,
		}
	}

	return statuschecker.ObjectStatusInProgress{
		Message: "Waiting for service catalog",
	}
}

func isScServiceInstanceReady(obj runtime.Object) statuschecker.ObjectStatusResult {
	var instance sc_v1b1.ServiceInstance
	if err := util.ConvertType(scV1B1Scheme, obj, &instance); err != nil {
		return statuschecker.ObjectStatusError{
			Error: err,
		}
	}
	readyCond := getServiceInstanceCondition(&instance, sc_v1b1.ServiceInstanceConditionReady)
	if readyCond != nil {
		switch readyCond.Status {
		case sc_v1b1.ConditionFalse:
			if !scNonErrorReasons.Has(readyCond.Reason) {
				// e.g. ProvisioningCallFailed
				return statuschecker.ObjectStatusError{
					Error:          errors.Errorf("%s: %s", readyCond.Reason, readyCond.Message),
					RetriableError: true,
					ExternalError:  true,
				}
			}
			return statuschecker.ObjectStatusInProgress{
				Message: fmt.Sprintf("%v: %v", readyCond.Reason, readyCond.Message),
			}
		case sc_v1b1.ConditionTrue:
			var msg []string
			if len(readyCond.Reason) != 0 {
				msg = append(msg, readyCond.Reason)
			}
			if len(readyCond.Message) != 0 {
				msg = append(msg, readyCond.Message)
			}
			return statuschecker.ObjectStatusReady{
				Message: strings.Join(msg, ": "),
			}
		default:
			return statuschecker.ObjectStatusUnknown{
				Details: fmt.Sprintf("status is %q", readyCond.Status),
			}
		}
	}
	failedCond := getServiceInstanceCondition(&instance, sc_v1b1.ServiceInstanceConditionFailed)
	if failedCond != nil && failedCond.Status == sc_v1b1.ConditionTrue {
		return statuschecker.ObjectStatusError{
			Error:         errors.Errorf("%s: %s", failedCond.Reason, failedCond.Message),
			ExternalError: true,
		}
	}

	return statuschecker.ObjectStatusInProgress{
		Message: "Waiting for service catalog",
	}
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
