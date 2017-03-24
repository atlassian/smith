package readychecker

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	unstructured_conversion "k8s.io/apimachinery/pkg/conversion/unstructured"
	extensions "k8s.io/client-go/pkg/apis/extensions/v1beta1"
)

var converter = unstructured_conversion.NewConverter(false)

// Works according to https://kubernetes.io/docs/user-guide/deployments/#the-status-of-a-deployment
// and k8s.io/kubernetes/pkg/client/unversioned/conditions.go:120 DeploymentHasDesiredReplicas()
func isDeploymentReady(obj *unstructured.Unstructured) (bool, error) {
	var deployment extensions.Deployment
	if err := converter.FromUnstructured(obj.Object, &deployment); err != nil {
		return false, err
	}

	replicas := int32(1) // Default value if not specified
	if deployment.Spec.Replicas != nil {
		replicas = *deployment.Spec.Replicas
	}

	return deployment.Status.ObservedGeneration >= deployment.Generation &&
		deployment.Status.UpdatedReplicas == replicas, nil
}
