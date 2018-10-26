package bundlec

import (
	"bytes"
	"crypto/sha256"
	"sort"

	"github.com/atlassian/smith"
	"github.com/atlassian/smith/pkg/util"
	"github.com/pkg/errors"
	apps_v1 "k8s.io/api/apps/v1"
	core_v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	envFromChecksumAnnotation = smith.Domain + "/envFromChecksum"
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

	hasEnvFrom := false
	for _, container := range deploymentSpec.Spec.Template.Spec.Containers {
		if len(container.EnvFrom) != 0 {
			hasEnvFrom = true
			break
		}
	}
	if !hasEnvFrom {
		return spec, nil
	}

	if v, ok := deploymentSpec.Spec.Template.ObjectMeta.Annotations[envFromChecksumAnnotation]; ok && v == disabled {
		return spec, nil
	}

	bytes, err := st.generateChecksum(deploymentSpec.Spec.Template, namespace)
	if err != nil {
		return nil, errors.Wrap(err, "failed to generate checksum")
	}

	// We don't need to do any comparisons here, a change to the checksum in the pod template annotation
	// will force the pod to update.
	checksum, err := generateChecksum(bytes)
	if err != nil {
		return nil, err
	}

	checksumHex := encodeChecksum(checksum)
	setDeploymentAnnotation(deploymentSpec, checksumHex)

	unstructuredSpec, err := util.RuntimeToUnstructured(deploymentSpec)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal back into unstructured")
	}

	return unstructuredSpec, nil
}

func (st *resourceSyncTask) generateChecksum(template core_v1.PodTemplateSpec, namespace string) ([]byte, error) {
	var buf bytes.Buffer

	for _, container := range template.Spec.Containers {
		for _, envFrom := range container.EnvFrom {
			if envFrom.SecretRef != nil {
				secretRef := envFrom.SecretRef

				gvk := core_v1.SchemeGroupVersion.WithKind("Secret")
				obj, exists, err := st.store.Get(gvk, namespace, secretRef.Name)
				if err != nil {
					return nil, errors.Wrapf(err, "failure retrieving %v %q referenced from envFrom", gvk.Kind, secretRef.Name)
				}
				if !exists {
					return nil, errors.Errorf("%v %q referenced from envFrom not found", gvk.Kind, secretRef.Name)
				}
				secret, ok := obj.(*core_v1.Secret)
				if !ok {
					return nil, errors.Errorf("failure casting %v %q referenced from envFrom", gvk.Kind, secretRef.Name)
				}

				keys := make([]string, 0, len(secret.Data))
				for k := range secret.Data {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				for _, k := range keys {
					secretData := secret.Data[k]
					hash := sha256.Sum256(secretData)
					_, err = buf.Write(hash[:])
					if err != nil {
						return nil, err
					}
				}
			}
			if envFrom.ConfigMapRef != nil {
				configMapRef := envFrom.ConfigMapRef

				gvk := core_v1.SchemeGroupVersion.WithKind("ConfigMap")
				obj, exists, err := st.store.Get(gvk, namespace, configMapRef.Name)
				if err != nil {
					return nil, errors.Wrapf(err, "failure retrieving %v %q referenced from envFrom", gvk.Kind, configMapRef.Name)
				}
				if !exists {
					return nil, errors.Errorf("%v %q referenced from envFrom not found", gvk.Kind, configMapRef.Name)
				}
				configMap, ok := obj.(*core_v1.ConfigMap)
				if !ok {
					return nil, errors.Errorf("failure casting %v %q referenced from envFrom", gvk.Kind, configMapRef.Name)
				}

				keys := make([]string, 0, len(configMap.Data))
				for k := range configMap.Data {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				for _, k := range keys {
					configData := configMap.Data[k]
					hash := sha256.Sum256([]byte(configData))
					_, err = buf.Write(hash[:])
					if err != nil {
						return nil, err
					}
				}
			}
		}
	}

	return buf.Bytes(), nil
}

func setDeploymentAnnotation(deploymentSpec *apps_v1.Deployment, checksum string) {
	if deploymentSpec.Spec.Template.ObjectMeta.Annotations == nil {
		deploymentSpec.Spec.Template.ObjectMeta.Annotations = make(map[string]string, 1)
	}

	deploymentSpec.Spec.Template.ObjectMeta.Annotations[envFromChecksumAnnotation] = checksum
}
