package builtin

import (
	"crypto/sha256"
	"encoding/hex"
	"hash"
	"strconv"
	"strings"

	"github.com/atlassian/smith"
	"github.com/atlassian/smith/pkg/speccheck"
	"github.com/atlassian/smith/pkg/util"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	apps_v1 "k8s.io/api/apps/v1"
	core_v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
)

const (
	// LastAppliedReplicasAnnotation is the name of annotation which stores last applied replicas for deployment
	LastAppliedReplicasAnnotation = smith.Domain + "/LastAppliedReplicas"
	EnvRefHashAnnotation          = smith.Domain + "/envRefHash"
)

type deployment struct {
}

func (d deployment) BeforeCreate(ctx *speccheck.Context, spec *unstructured.Unstructured) (runtime.Object /*updatedSpec*/, error) {
	var deploymentSpec apps_v1.Deployment
	if err := util.ConvertType(appsV1Scheme, spec, &deploymentSpec); err != nil {
		return nil, err
	}

	if deploymentSpec.Annotations == nil {
		deploymentSpec.Annotations = make(map[string]string)
	}
	d.setLastAppliedReplicasAnnotation(ctx, &deploymentSpec, nil)
	err := d.setConfigurationHashAnnotation(ctx, &deploymentSpec)
	if err != nil {
		return nil, err
	}
	return &deploymentSpec, nil
}

func (d deployment) ApplySpec(ctx *speccheck.Context, spec, actual *unstructured.Unstructured) (runtime.Object, error) {
	var deploymentSpec apps_v1.Deployment
	if err := util.ConvertType(appsV1Scheme, spec, &deploymentSpec); err != nil {
		return nil, err
	}
	var deploymentActual apps_v1.Deployment
	if err := util.ConvertType(appsV1Scheme, actual, &deploymentActual); err != nil {
		return nil, err
	}

	deploymentSpec.Spec.Template.Spec.DeprecatedServiceAccount = deploymentSpec.Spec.Template.Spec.ServiceAccountName

	if deploymentSpec.Annotations == nil {
		deploymentSpec.Annotations = make(map[string]string)
	}

	d.setLastAppliedReplicasAnnotation(ctx, &deploymentSpec, &deploymentActual)
	err := d.setConfigurationHashAnnotation(ctx, &deploymentSpec)
	if err != nil {
		return nil, err
	}

	return &deploymentSpec, nil
}

// setLastAppliedReplicasAnnotation updates replicas based on LastAppliedReplicas annotation and running config
// to avoid conflicts with other controllers like HPA.
// actual may be nil.
func (deployment) setLastAppliedReplicasAnnotation(ctx *speccheck.Context, spec, actual *apps_v1.Deployment) {
	if spec.Spec.Replicas == nil {
		var one int32 = 1
		spec.Spec.Replicas = &one
	}

	specReplicas := *spec.Spec.Replicas
	if actual == nil {
		// add LastAppliedReplicas annotation if it doesn't exist
		spec.Annotations[LastAppliedReplicasAnnotation] = strconv.Itoa(int(specReplicas))
		return
	}

	lastAppliedReplicasConf, ok := actual.Annotations[LastAppliedReplicasAnnotation]
	if !ok {
		// add LastAppliedReplicas annotation if it doesn't exist
		spec.Annotations[LastAppliedReplicasAnnotation] = strconv.Itoa(int(specReplicas))
		return
	}

	// Parse last applied replicas from running config's annotation
	// overrides with current replicas inside spec if parsing failure
	lastAppliedReplicas, err := strconv.Atoi(strings.TrimSpace(lastAppliedReplicasConf))
	if err != nil {
		ctx.Logger.Warn("Overriding last applied replicas annotation due to parsing failure", zap.Error(err))
		spec.Annotations[LastAppliedReplicasAnnotation] = strconv.Itoa(int(specReplicas))
		return
	}

	if specReplicas == int32(lastAppliedReplicas) {
		// spec not changed => use actual running config if it exists
		// since it might be updated by other controller like HPA
		// otherwise use spec replicas config
		if actual.Spec.Replicas != nil {
			*spec.Spec.Replicas = *actual.Spec.Replicas
		}
	} else {
		// spec changed => update annotations and use spec replicas config
		spec.Annotations[LastAppliedReplicasAnnotation] = strconv.Itoa(int(specReplicas))
	}
}

// works around https://github.com/kubernetes/kubernetes/issues/22368
func (d deployment) setConfigurationHashAnnotation(ctx *speccheck.Context, spec *apps_v1.Deployment) error {
	if spec.Spec.Template.Annotations[EnvRefHashAnnotation] == Disabled {
		return nil
	}

	hashBytes, err := d.generateHash(ctx, spec)
	if err != nil {
		return errors.Wrap(err, "failed to generate checksum")
	}

	if spec.Spec.Template.Annotations == nil {
		spec.Spec.Template.Annotations = make(map[string]string, 1)
	}
	spec.Spec.Template.Annotations[EnvRefHashAnnotation] = hex.EncodeToString(hashBytes)
	return nil
}

func (d deployment) generateHash(ctx *speccheck.Context, spec *apps_v1.Deployment) ([]byte, error) {
	hasher := sha256.New()

	err := d.generateHashForContainers(ctx.Store, spec.Namespace, spec.Spec.Template.Spec.Containers, hasher)
	if err != nil {
		return nil, err
	}
	err = d.generateHashForContainers(ctx.Store, spec.Namespace, spec.Spec.Template.Spec.InitContainers, hasher)
	if err != nil {
		return nil, err
	}
	return hasher.Sum(nil), nil
}

func (deployment) generateHashForContainers(store speccheck.Store, namespace string, containers []core_v1.Container, hasher hash.Hash) error {
	for _, container := range containers {
		for _, envFrom := range container.EnvFrom {
			secretRef := envFrom.SecretRef
			if secretRef != nil {
				err := speccheck.HashSecretRef(store, namespace, secretRef.Name, sets.NewString(), secretRef.Optional, hasher)
				if err != nil {
					return err
				}
			}

			configMapRef := envFrom.ConfigMapRef
			if configMapRef != nil {
				err := speccheck.HashConfigMapRef(store, namespace, configMapRef.Name, sets.NewString(), configMapRef.Optional, hasher)
				if err != nil {
					return err
				}
			}
		}
		for _, env := range container.Env {
			if env.ValueFrom == nil {
				continue
			}

			secretKeyRef := env.ValueFrom.SecretKeyRef
			if secretKeyRef != nil {
				err := speccheck.HashSecretRef(store, namespace, secretKeyRef.Name, sets.NewString(secretKeyRef.Key), secretKeyRef.Optional, hasher)
				if err != nil {
					return err
				}
			}

			configMapKeyRef := env.ValueFrom.ConfigMapKeyRef
			if configMapKeyRef != nil {
				err := speccheck.HashConfigMapRef(store, namespace, configMapKeyRef.Name, sets.NewString(configMapKeyRef.Key), configMapKeyRef.Optional, hasher)
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}
