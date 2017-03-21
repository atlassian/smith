package readychecker

import (
	"github.com/atlassian/smith/pkg/resources"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	extensions_v1beta1 "k8s.io/client-go/pkg/apis/extensions/v1beta1"
)

// Works according to https://kubernetes.io/docs/user-guide/deployments/#the-status-of-a-deployment
// and k8s.io/kubernetes/pkg/client/unversioned/conditions.go:120 DeploymentHasDesiredReplicas()
func isDeploymentReady(obj *unstructured.Unstructured) (bool, error) {
	var deployment extensions_v1beta1.Deployment
	if err := resources.UnstructuredToType(obj, &deployment); err != nil {
		return false, err
	}

	replicas := int32(1) // Default value if not specified
	if deployment.Spec.Replicas != nil {
		replicas = *deployment.Spec.Replicas
	}

	return deployment.Status.ObservedGeneration >= deployment.Generation &&
		deployment.Status.UpdatedReplicas == replicas, nil
}
