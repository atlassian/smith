package bundlec

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/atlassian/smith"
	"github.com/atlassian/smith/pkg/util"
	"github.com/pkg/errors"
	apps_v1 "k8s.io/api/apps/v1"
	core_v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
)

const (
	envRefHashAnnotation = smith.Domain + "/envRefHash"
)

// works around https://github.com/kubernetes/kubernetes/issues/22368
func (st *resourceSyncTask) forceDeploymentUpdates(spec *unstructured.Unstructured, actual runtime.Object, namespace string) (specRet *unstructured.Unstructured, e error) {
	gvk := spec.GroupVersionKind()

	if gvk.Group == apps_v1.GroupName && gvk.Kind == "Deployment" {
		return st.processDeployment(spec, actual, namespace)
	}

	return spec, nil
}

func (st *resourceSyncTask) processDeployment(spec *unstructured.Unstructured, actual runtime.Object, namespace string) (specRet *unstructured.Unstructured, e error) {
	deploymentSpec := &apps_v1.Deployment{}

	if err := util.ConvertType(st.scheme, spec, deploymentSpec); err != nil {
		return nil, errors.Wrap(err, "failure to parse Deployment")
	}

	if v, ok := deploymentSpec.Spec.Template.ObjectMeta.Annotations[envRefHashAnnotation]; ok && v == disabled {
		return spec, nil
	}

	bytes, err := st.generateHash(deploymentSpec.Spec.Template, namespace)
	if err != nil {
		return nil, errors.Wrap(err, "failed to generate checksum")
	}

	setDeploymentAnnotation(deploymentSpec, hex.EncodeToString(bytes))

	unstructuredSpec, err := util.RuntimeToUnstructured(deploymentSpec)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal back into unstructured")
	}

	return unstructuredSpec, nil
}

func (st *resourceSyncTask) generateHash(template core_v1.PodTemplateSpec, namespace string) ([]byte, error) {
	hash := sha256.New()

	containers := make([]core_v1.Container, 0, len(template.Spec.Containers)+len(template.Spec.InitContainers))
	containers = append(containers, template.Spec.Containers...)
	containers = append(containers, template.Spec.InitContainers...)

	for _, container := range containers {
		for _, envFrom := range container.EnvFrom {
			secretRef := envFrom.SecretRef
			if secretRef != nil {
				err := st.hashSecretRef(secretRef.Name, namespace, sets.NewString(), secretRef.Optional, hash)
				if err != nil {
					return nil, err
				}
			}

			configMapRef := envFrom.ConfigMapRef
			if configMapRef != nil {
				err := st.hashConfigMapRef(configMapRef.Name, namespace, sets.NewString(), configMapRef.Optional, hash)
				if err != nil {
					return nil, err
				}
			}
		}
		for _, env := range container.Env {
			if env.ValueFrom == nil {
				continue
			}

			secretKeyRef := env.ValueFrom.SecretKeyRef
			if secretKeyRef != nil {
				err := st.hashSecretRef(secretKeyRef.Name, namespace, sets.NewString(secretKeyRef.Key), secretKeyRef.Optional, hash)
				if err != nil {
					return nil, err
				}
			}

			configMapKeyRef := env.ValueFrom.ConfigMapKeyRef
			if configMapKeyRef != nil {
				err := st.hashConfigMapRef(configMapKeyRef.Name, namespace, sets.NewString(configMapKeyRef.Key), configMapKeyRef.Optional, hash)
				if err != nil {
					return nil, err
				}
			}
		}
	}

	return hash.Sum(nil), nil
}

func setDeploymentAnnotation(deploymentSpec *apps_v1.Deployment, hash string) {
	if deploymentSpec.Spec.Template.ObjectMeta.Annotations == nil {
		deploymentSpec.Spec.Template.ObjectMeta.Annotations = make(map[string]string, 1)
	}

	deploymentSpec.Spec.Template.ObjectMeta.Annotations[envRefHashAnnotation] = hash
}
