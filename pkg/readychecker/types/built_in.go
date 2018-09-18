package types

import (
	"strconv"

	"github.com/atlassian/smith/pkg/readychecker"
	"github.com/atlassian/smith/pkg/util"
	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	"github.com/pkg/errors"
	apps "k8s.io/api/apps/v1"
	apps_v1 "k8s.io/api/apps/v1"
	core_v1 "k8s.io/api/core/v1"
	ext_v1b1 "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)


var (
	MainKnownTypes = map[schema.GroupKind]readychecker.IsObjectReady{
		{Group: core_v1.GroupName, Kind: "ConfigMap"}:      alwaysReady,
		{Group: core_v1.GroupName, Kind: "Secret"}:         alwaysReady,
		{Group: core_v1.GroupName, Kind: "Service"}:        alwaysReady,
		{Group: core_v1.GroupName, Kind: "ServiceAccount"}: alwaysReady,
		{Group: apps_v1.GroupName, Kind: "Deployment"}:     isDeploymentReady,
		{Group: ext_v1b1.GroupName, Kind: "Ingress"}:       alwaysReady,
	}
	ServiceCatalogKnownTypes = map[schema.GroupKind]readychecker.IsObjectReady{
		{Group: sc_v1b1.GroupName, Kind: "ServiceBinding"}:  isScServiceBindingReady,
		{Group: sc_v1b1.GroupName, Kind: "ServiceInstance"}: isScServiceInstanceReady,
	}
	appsV1Scheme = runtime.NewScheme()
	scV1B1Scheme = runtime.NewScheme()
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
}

func alwaysReady(_ runtime.Object) (isReady, retriableError bool, e error) {
	return true, false, nil
}

func getDeploymentRevision(deployment *apps_v1.Deployment) (int64, error) {
	const RevisionAnnotation = "deployment.kubernetes.io/revision"

	v, ok := deployment.GetAnnotations()[RevisionAnnotation]
	if !ok {
		return 0, nil
	}
	return strconv.ParseInt(v, 10, 64)
}

func getDeploymentCondition(status apps.DeploymentStatus, condType apps.DeploymentConditionType) *apps.DeploymentCondition {
	for i := range status.Conditions {
		c := status.Conditions[i]
		if c.Type == condType {
			return &c
		}
	}
	return nil
}

// Works according to https://kubernetes.io/docs/user-guide/deployments/#the-status-of-a-deployment
// and k8s.io/kubernetes/pkg/kubectl/rollout_status.go#L67 Status()
func isDeploymentReady(obj runtime.Object) (isReady, retriableError bool, e error) {
	const TimedOutReason     = "ProgressDeadlineExceeded"

	var deployment apps_v1.Deployment
	if err := util.ConvertType(appsV1Scheme, obj, &deployment); err != nil {
		return false, false, err
	}

	revision := int64(1)
	if revision > 0 {
		deploymentRev, err := getDeploymentRevision(&deployment)
		if err != nil {
			return false, true, errors.Errorf("cannot get the revision of deployment %q: %v", deployment.Name, err)
		}
		if revision != deploymentRev {
			return false, false, errors.Errorf("desired revision (%d) is different from the running revision (%d)", revision, deploymentRev)
		}
	}

	if deployment.Generation <= deployment.Status.ObservedGeneration {
		progressingCond := getDeploymentCondition(deployment.Status, apps_v1.DeploymentProgressing)
		if progressingCond != nil && progressingCond.Reason == TimedOutReason {
			return false, false, errors.Errorf("deployment %q exceeded its progress deadline", deployment.Name)
		}
		if deployment.Spec.Replicas != nil && deployment.Status.UpdatedReplicas < *deployment.Spec.Replicas {
			return false, true, nil
		}
		if deployment.Status.Replicas > deployment.Status.UpdatedReplicas {
			return false, true, nil
		}
		if deployment.Status.AvailableReplicas < deployment.Status.UpdatedReplicas {
			return false, true, nil
		}
		return true, false, nil
	}
	return false, true, nil

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
