package builtin

import (
	"strconv"
	"strings"

	"github.com/atlassian/smith"
	"github.com/atlassian/smith/pkg/speccheck"
	"github.com/atlassian/smith/pkg/util"
	"go.uber.org/zap"
	apps_v1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	// LastAppliedReplicasAnnotation is the name of annotation which stores last applied replicas for deployment
	LastAppliedReplicasAnnotation = smith.Domain + "/LastAppliedReplicas"
)

type deployment struct {
}

func (deployment) ApplySpec(ctx *speccheck.Context, spec, actual *unstructured.Unstructured) (runtime.Object, error) {
	var deploymentSpec apps_v1.Deployment
	if err := util.ConvertType(appsV1Scheme, spec, &deploymentSpec); err != nil {
		return nil, err
	}
	var deploymentActual apps_v1.Deployment
	if err := util.ConvertType(appsV1Scheme, actual, &deploymentActual); err != nil {
		return nil, err
	}

	deploymentSpec.Spec.Template.Spec.DeprecatedServiceAccount = deploymentSpec.Spec.Template.Spec.ServiceAccountName

	/*
		update replicas based on LastAppliedReplicas annotation and running config
		to avoid conflicts with other controllers like HPA
	*/

	if deploymentSpec.Spec.Replicas == nil {
		return util.RuntimeToUnstructured(&deploymentSpec)
	}

	if deploymentSpec.Annotations == nil {
		deploymentSpec.Annotations = make(map[string]string)
	}

	lastAppliedReplicasConf, ok := deploymentActual.Annotations[LastAppliedReplicasAnnotation]
	if !ok {
		// add LastAppliedReplicas annotation if it doesn't exist
		deploymentSpec.Annotations[LastAppliedReplicasAnnotation] = strconv.Itoa(int(*deploymentSpec.Spec.Replicas))
		return util.RuntimeToUnstructured(&deploymentSpec)
	}

	// Parse last applied replicas from running config's annotation
	// overrides with current replicas inside spec if parsing failure
	lastAppliedReplicas, err := strconv.Atoi(strings.TrimSpace(lastAppliedReplicasConf))
	if err != nil {
		ctx.Logger.Warn("overriding last applied replicas annotation due to parsing failure", zap.Error(err))
		deploymentSpec.Annotations[LastAppliedReplicasAnnotation] = strconv.Itoa(int(*deploymentSpec.Spec.Replicas))
		return util.RuntimeToUnstructured(&deploymentSpec)
	}

	// spec changed => update annotations and use spec replicas config
	if *deploymentSpec.Spec.Replicas != int32(lastAppliedReplicas) {
		deploymentSpec.Annotations[LastAppliedReplicasAnnotation] = strconv.Itoa(int(*deploymentSpec.Spec.Replicas))
	} else {
		// spec not changed => use actual running config if it exists
		// since it might be updated by other controller like HPA
		// otherwise use spec replicas config
		if deploymentActual.Spec.Replicas != nil {
			*deploymentSpec.Spec.Replicas = *deploymentActual.Spec.Replicas
		}
	}

	return util.RuntimeToUnstructured(&deploymentSpec)
}
