package bundlec

import (
	"testing"

	"github.com/atlassian/smith"
	"github.com/atlassian/smith/pkg/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apps_v1 "k8s.io/api/apps/v1"
	core_v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	deploymentTestNamespace     = "ns"
	deploymentTestAnnotationKey = smith.Domain + "/envFromChecksum"
)

func deploymentUnmarshal(t *testing.T, spec *unstructured.Unstructured) *apps_v1.Deployment {
	var deploymentSpec apps_v1.Deployment
	err := util.ConvertType(scheme(t), spec, &deploymentSpec)
	require.NoError(t, err)
	return &deploymentSpec
}

func TestAddsChecksumToDeploymentSpec(t *testing.T) {
	t.Parallel()

	deploymentSpec := apps_v1.Deployment{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: apps_v1.SchemeGroupVersion.String(),
		},
		Spec: apps_v1.DeploymentSpec{
			Template: core_v1.PodTemplateSpec{
				Spec: core_v1.PodSpec{
					Containers: []core_v1.Container{
						core_v1.Container{}, // empty EnvFrom
						core_v1.Container{
							EnvFrom: []core_v1.EnvFromSource{
								core_v1.EnvFromSource{
									SecretRef: &core_v1.SecretEnvSource{
										LocalObjectReference: core_v1.LocalObjectReference{
											Name: "secret1",
										},
									},
								},
							},
						},
						core_v1.Container{
							EnvFrom: []core_v1.EnvFromSource{
								core_v1.EnvFromSource{
									SecretRef: &core_v1.SecretEnvSource{
										LocalObjectReference: core_v1.LocalObjectReference{
											Name: "secret1",
										},
									},
								},
								core_v1.EnvFromSource{
									SecretRef: &core_v1.SecretEnvSource{
										LocalObjectReference: core_v1.LocalObjectReference{
											Name: "secret2",
										},
									},
								},
								core_v1.EnvFromSource{
									ConfigMapRef: &core_v1.ConfigMapEnvSource{
										LocalObjectReference: core_v1.LocalObjectReference{
											Name: "configmap1",
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	spec := runtimeToUnstructured(t, &deploymentSpec)

	rst := resourceSyncTask{
		store: fakeStore{
			responses: map[string]runtime.Object{
				"secret1": &core_v1.Secret{
					TypeMeta: meta_v1.TypeMeta{
						Kind:       "Secret",
						APIVersion: "v1",
					},
					ObjectMeta: meta_v1.ObjectMeta{
						Name: "secret1",
					},
					Data: map[string][]byte{
						"parameters": []byte(`{
							"secretEnvVars": {
								"a": "1",
								"b": "2"
							}
						}`),
					},
				},
				"secret2": &core_v1.Secret{
					TypeMeta: meta_v1.TypeMeta{
						Kind:       "Secret",
						APIVersion: "v1",
					},
					ObjectMeta: meta_v1.ObjectMeta{
						Name: "secret2",
					},
					Data: map[string][]byte{
						"parameters": []byte(`{
							"iamRole": "some-role"
							}
						}`),
					},
				},
				"configmap1": &core_v1.ConfigMap{
					TypeMeta: meta_v1.TypeMeta{
						Kind:       "ConfigMap",
						APIVersion: "v1",
					},
					ObjectMeta: meta_v1.ObjectMeta{
						Name: "configmap1",
					},
					Data: map[string]string{
						"a": "b",
						"c": "d",
					},
				},
			},
		},
		scheme: scheme(t),
	}

	updatedSpec, err := rst.forceDeploymentUpdates(spec, nil, deploymentTestNamespace)
	require.NoError(t, err)

	deploymentCheck := deploymentUnmarshal(t, updatedSpec)

	assert.Contains(t, deploymentCheck.Spec.Template.ObjectMeta.Annotations, deploymentTestAnnotationKey)
}

func TestNoAnnotationForEmptyEnvFrom(t *testing.T) {
	t.Parallel()

	deploymentSpec := apps_v1.Deployment{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: apps_v1.SchemeGroupVersion.String(),
		},
		Spec: apps_v1.DeploymentSpec{},
	}

	spec := runtimeToUnstructured(t, &deploymentSpec)

	rst := resourceSyncTask{
		scheme: scheme(t),
	}

	updatedSpec, err := rst.forceDeploymentUpdates(spec, nil, deploymentTestNamespace)
	require.NoError(t, err)

	assert.True(t, equality.Semantic.DeepEqual(spec.Object, updatedSpec.Object))
}

func TestDeploymentAnnotationExplicitlyDisabled(t *testing.T) {
	t.Parallel()

	deploymentSpec := apps_v1.Deployment{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: apps_v1.SchemeGroupVersion.String(),
		},
		Spec: apps_v1.DeploymentSpec{
			Template: core_v1.PodTemplateSpec{
				ObjectMeta: meta_v1.ObjectMeta{
					Annotations: map[string]string{
						deploymentTestAnnotationKey: "disabled",
					},
				},
				Spec: core_v1.PodSpec{
					Containers: []core_v1.Container{
						core_v1.Container{
							EnvFrom: []core_v1.EnvFromSource{
								core_v1.EnvFromSource{
									SecretRef: &core_v1.SecretEnvSource{
										LocalObjectReference: core_v1.LocalObjectReference{
											Name: "secret1",
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	spec := runtimeToUnstructured(t, &deploymentSpec)

	rst := resourceSyncTask{
		scheme: scheme(t),
	}

	updatedSpec, err := rst.forceDeploymentUpdates(spec, nil, deploymentTestNamespace)
	require.NoError(t, err)

	deploymentCheck := deploymentUnmarshal(t, updatedSpec)

	assert.Contains(t, deploymentCheck.Spec.Template.ObjectMeta.Annotations, deploymentTestAnnotationKey)
	assert.Contains(t, deploymentCheck.Spec.Template.Annotations, deploymentTestAnnotationKey)
	assert.Equal(t, deploymentCheck.Spec.Template.Annotations[deploymentTestAnnotationKey], "disabled")
}

func TestUserEnteredAnnotationInDeploymentNoRefs(t *testing.T) {
	t.Parallel()

	expectedAnnotationValue := "mashingonthekeyboard"
	deploymentSpec := apps_v1.Deployment{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: apps_v1.SchemeGroupVersion.String(),
		},
		Spec: apps_v1.DeploymentSpec{
			Template: core_v1.PodTemplateSpec{
				ObjectMeta: meta_v1.ObjectMeta{
					Annotations: map[string]string{
						deploymentTestAnnotationKey: expectedAnnotationValue,
					},
				},
				Spec: core_v1.PodSpec{},
			},
		},
	}

	spec := runtimeToUnstructured(t, &deploymentSpec)

	rst := resourceSyncTask{
		scheme: scheme(t),
	}

	updatedSpec, err := rst.forceDeploymentUpdates(spec, nil, deploymentTestNamespace)
	require.NoError(t, err)

	deploymentCheck := deploymentUnmarshal(t, updatedSpec)
	assert.Contains(t, deploymentCheck.Spec.Template.ObjectMeta.Annotations, deploymentTestAnnotationKey)
	assert.Equal(t, deploymentCheck.Spec.Template.ObjectMeta.Annotations[deploymentTestAnnotationKey], expectedAnnotationValue)
}

func TestUserEnteredAnnotationInDeploymentWithRefs(t *testing.T) {
	t.Parallel()

	expectedAnnotationValue := "mashingonthekeyboard"
	deploymentSpec := apps_v1.Deployment{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: apps_v1.SchemeGroupVersion.String(),
		},
		Spec: apps_v1.DeploymentSpec{
			Template: core_v1.PodTemplateSpec{
				ObjectMeta: meta_v1.ObjectMeta{
					Annotations: map[string]string{
						deploymentTestAnnotationKey: expectedAnnotationValue,
					},
				},
				Spec: core_v1.PodSpec{
					Containers: []core_v1.Container{
						core_v1.Container{
							EnvFrom: []core_v1.EnvFromSource{
								core_v1.EnvFromSource{
									SecretRef: &core_v1.SecretEnvSource{
										LocalObjectReference: core_v1.LocalObjectReference{
											Name: "secret1",
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	spec := runtimeToUnstructured(t, &deploymentSpec)

	rst := resourceSyncTask{
		store: fakeStore{
			responses: map[string]runtime.Object{
				"secret1": &core_v1.Secret{
					TypeMeta: meta_v1.TypeMeta{
						Kind:       "Secret",
						APIVersion: "v1",
					},
					ObjectMeta: meta_v1.ObjectMeta{
						Name: "secret1",
					},
					Data: map[string][]byte{
						"parameters": []byte(`{
							"secretEnvVars": {
								"a": "1",
								"b": "2"
							}
						}`),
					},
				},
			},
		},
		scheme: scheme(t),
	}

	updatedSpec, err := rst.forceDeploymentUpdates(spec, nil, deploymentTestNamespace)
	require.NoError(t, err)

	deploymentCheck := deploymentUnmarshal(t, updatedSpec)
	assert.Contains(t, deploymentCheck.Spec.Template.ObjectMeta.Annotations, deploymentTestAnnotationKey)

	// The annotation is no longer equal
	assert.NotEqual(t, deploymentCheck.Spec.Template.ObjectMeta.Annotations[deploymentTestAnnotationKey], expectedAnnotationValue)
}

func TestDeploymentUpdatedSecrets(t *testing.T) {
	t.Parallel()

	deploymentSpec := apps_v1.Deployment{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: apps_v1.SchemeGroupVersion.String(),
		},
		Spec: apps_v1.DeploymentSpec{
			Template: core_v1.PodTemplateSpec{
				Spec: core_v1.PodSpec{
					Containers: []core_v1.Container{
						core_v1.Container{
							EnvFrom: []core_v1.EnvFromSource{
								core_v1.EnvFromSource{
									SecretRef: &core_v1.SecretEnvSource{
										LocalObjectReference: core_v1.LocalObjectReference{
											Name: "secret1",
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	spec := runtimeToUnstructured(t, &deploymentSpec)

	allResponses := map[string]runtime.Object{
		"secret1": &core_v1.Secret{
			TypeMeta: meta_v1.TypeMeta{
				Kind:       "Secret",
				APIVersion: "v1",
			},
			ObjectMeta: meta_v1.ObjectMeta{
				Name: "secret1",
			},
			Data: map[string][]byte{
				"parameters": []byte(`{
							"secretEnvVars": {
								"a": "1",
								"b": "2"
							}
						}`),
			},
		},
		"secret2": &core_v1.Secret{
			TypeMeta: meta_v1.TypeMeta{
				Kind:       "Secret",
				APIVersion: "v1",
			},
			ObjectMeta: meta_v1.ObjectMeta{
				Name: "secret2",
			},
			Data: map[string][]byte{
				"parameters": []byte(`{
							"iamRole": "some-role"
							}
						}`),
			},
		},
	}
	rst := resourceSyncTask{
		store: fakeStore{
			responses: allResponses,
		},
		scheme: scheme(t),
	}

	updatedSpec, err := rst.forceDeploymentUpdates(spec, nil, deploymentTestNamespace)
	require.NoError(t, err)

	deploymentCheck := deploymentUnmarshal(t, updatedSpec)
	assert.Contains(t, deploymentCheck.Spec.Template.ObjectMeta.Annotations, deploymentTestAnnotationKey)

	firstChecksum := deploymentCheck.Spec.Template.ObjectMeta.Annotations

	allResponses["secret1"] = allResponses["secret2"]
	rstUpdatedMocks := resourceSyncTask{
		store: fakeStore{
			responses: allResponses,
		},
		scheme: scheme(t),
	}

	secondUpdate, err := rstUpdatedMocks.forceDeploymentUpdates(spec, nil, deploymentTestNamespace)
	require.NoError(t, err)

	secondDeploymentCheck := deploymentUnmarshal(t, secondUpdate)
	assert.Contains(t, secondDeploymentCheck.Spec.Template.ObjectMeta.Annotations, deploymentTestAnnotationKey)
	assert.NotEqual(t, secondDeploymentCheck.Spec.Template.ObjectMeta.Annotations[deploymentTestAnnotationKey], firstChecksum)
}
