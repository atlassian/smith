package builtin

import (
	"testing"

	"github.com/atlassian/smith/pkg/specchecker"
	speccheckertesting "github.com/atlassian/smith/pkg/specchecker/testing"
	"github.com/atlassian/smith/pkg/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	apps_v1 "k8s.io/api/apps/v1"
	core_v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/diff"
)

const (
	nullSha256 = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
)

func TestAddsHashToDeploymentSpecForEnvFrom(t *testing.T) {
	t.Parallel()

	deploymentSpec := apps_v1.Deployment{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: apps_v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: testNs,
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

	store := speccheckertesting.FakeStore{
		Namespace: testNs,
		Responses: map[string]runtime.Object{
			"secret1": &core_v1.Secret{
				TypeMeta: meta_v1.TypeMeta{
					Kind:       "Secret",
					APIVersion: "v1",
				},
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "secret1",
					Namespace: testNs,
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
					Name:      "secret2",
					Namespace: testNs,
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
					Name:      "configmap1",
					Namespace: testNs,
				},
				Data: map[string]string{
					"a": "b",
					"c": "d",
				},
			},
		},
	}
	logger := zaptest.NewLogger(t)
	defer logger.Sync() // nolint: errcheck

	updatedSpec, err := deployment{}.BeforeCreate(&specchecker.Context{Logger: logger, Store: store}, spec)
	require.NoError(t, err)

	deploymentCheck := updatedSpec.(*apps_v1.Deployment)

	require.Contains(t, deploymentCheck.Spec.Template.Annotations, EnvRefHashAnnotation)
	assert.NotEmpty(t, deploymentCheck.Spec.Template.Annotations[EnvRefHashAnnotation])
}

func TestAddsHashToDeploymentSpecForInitContainersEnvFrom(t *testing.T) {
	t.Parallel()

	deploymentSpec := apps_v1.Deployment{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: apps_v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: testNs,
		},
		Spec: apps_v1.DeploymentSpec{
			Template: core_v1.PodTemplateSpec{
				Spec: core_v1.PodSpec{
					InitContainers: []core_v1.Container{
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

	store := speccheckertesting.FakeStore{
		Namespace: testNs,
		Responses: map[string]runtime.Object{
			"secret1": &core_v1.Secret{
				TypeMeta: meta_v1.TypeMeta{
					Kind:       "Secret",
					APIVersion: "v1",
				},
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "secret1",
					Namespace: testNs,
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
					Name:      "secret2",
					Namespace: testNs,
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
					Name:      "configmap1",
					Namespace: testNs,
				},
				Data: map[string]string{
					"a": "b",
					"c": "d",
				},
			},
		},
	}

	logger := zaptest.NewLogger(t)
	defer logger.Sync() // nolint: errcheck

	updatedSpec, err := deployment{}.BeforeCreate(&specchecker.Context{Logger: logger, Store: store}, spec)
	require.NoError(t, err)

	deploymentCheck := updatedSpec.(*apps_v1.Deployment)

	require.Contains(t, deploymentCheck.Spec.Template.Annotations, EnvRefHashAnnotation)
	assert.NotEmpty(t, deploymentCheck.Spec.Template.Annotations[EnvRefHashAnnotation])
}

func TestAddsHashToDeploymentSpecForEnv(t *testing.T) {
	t.Parallel()

	deploymentSpec := apps_v1.Deployment{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: apps_v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: testNs,
		},
		Spec: apps_v1.DeploymentSpec{
			Template: core_v1.PodTemplateSpec{
				Spec: core_v1.PodSpec{
					Containers: []core_v1.Container{
						core_v1.Container{}, // empty EnvFrom
						core_v1.Container{
							Env: []core_v1.EnvVar{
								core_v1.EnvVar{
									Name: "blah2",
									ValueFrom: &core_v1.EnvVarSource{
										SecretKeyRef: &core_v1.SecretKeySelector{
											Key: "parameters",
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
		},
	}

	spec := runtimeToUnstructured(t, &deploymentSpec)

	store := speccheckertesting.FakeStore{
		Namespace: testNs,
		Responses: map[string]runtime.Object{
			"secret1": &core_v1.Secret{
				TypeMeta: meta_v1.TypeMeta{
					Kind:       "Secret",
					APIVersion: "v1",
				},
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "secret1",
					Namespace: testNs,
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
	}

	logger := zaptest.NewLogger(t)
	defer logger.Sync() // nolint: errcheck

	updatedSpec, err := deployment{}.BeforeCreate(&specchecker.Context{Logger: logger, Store: store}, spec)
	require.NoError(t, err)

	deploymentCheck := updatedSpec.(*apps_v1.Deployment)

	require.Contains(t, deploymentCheck.Spec.Template.Annotations, EnvRefHashAnnotation)
	assert.NotEmpty(t, deploymentCheck.Spec.Template.Annotations[EnvRefHashAnnotation])
}

func TestHashNotIgnoredForNonExistingKey(t *testing.T) {
	t.Parallel()

	deploymentSpec := apps_v1.Deployment{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: apps_v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: testNs,
		},
		Spec: apps_v1.DeploymentSpec{
			Template: core_v1.PodTemplateSpec{
				Spec: core_v1.PodSpec{
					Containers: []core_v1.Container{
						core_v1.Container{}, // empty EnvFrom
						core_v1.Container{
							Env: []core_v1.EnvVar{
								core_v1.EnvVar{
									Name: "blah2",
									ValueFrom: &core_v1.EnvVarSource{
										SecretKeyRef: &core_v1.SecretKeySelector{
											Key: "notexistingkey",
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
		},
	}

	spec := runtimeToUnstructured(t, &deploymentSpec)

	store := speccheckertesting.FakeStore{
		Namespace: testNs,
		Responses: map[string]runtime.Object{
			"secret1": &core_v1.Secret{
				TypeMeta: meta_v1.TypeMeta{
					Kind:       "Secret",
					APIVersion: "v1",
				},
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "secret1",
					Namespace: testNs,
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
	}

	logger := zaptest.NewLogger(t)
	defer logger.Sync() // nolint: errcheck

	_, err := deployment{}.BeforeCreate(&specchecker.Context{Logger: logger, Store: store}, spec)
	require.Error(t, err)
}

func TestHashIgnoredForOptionalNonExistingKey(t *testing.T) {
	t.Parallel()

	trueVal := true
	deploymentSpec := apps_v1.Deployment{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: apps_v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: testNs,
		},
		Spec: apps_v1.DeploymentSpec{
			Template: core_v1.PodTemplateSpec{
				Spec: core_v1.PodSpec{
					Containers: []core_v1.Container{
						core_v1.Container{}, // empty EnvFrom
						core_v1.Container{
							Env: []core_v1.EnvVar{
								core_v1.EnvVar{
									Name: "blah2",
									ValueFrom: &core_v1.EnvVarSource{
										SecretKeyRef: &core_v1.SecretKeySelector{
											Key: "notexistingkey",
											LocalObjectReference: core_v1.LocalObjectReference{
												Name: "secret1",
											},
											Optional: &trueVal,
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

	store := speccheckertesting.FakeStore{
		Namespace: testNs,
		Responses: map[string]runtime.Object{
			"secret1": &core_v1.Secret{
				TypeMeta: meta_v1.TypeMeta{
					Kind:       "Secret",
					APIVersion: "v1",
				},
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "secret1",
					Namespace: testNs,
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
	}
	logger := zaptest.NewLogger(t)
	defer logger.Sync() // nolint: errcheck

	updatedSpec, err := deployment{}.BeforeCreate(&specchecker.Context{Logger: logger, Store: store}, spec)
	require.NoError(t, err)

	deploymentCheck := updatedSpec.(*apps_v1.Deployment)

	require.Contains(t, deploymentCheck.Spec.Template.Annotations, EnvRefHashAnnotation)
	assert.Equal(t, nullSha256, deploymentCheck.Spec.Template.Annotations[EnvRefHashAnnotation])
}

func TestHashIgnoredForOptionalNonExistingSecret(t *testing.T) {
	t.Parallel()

	trueVal := true
	deploymentSpec := apps_v1.Deployment{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: apps_v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: testNs,
		},
		Spec: apps_v1.DeploymentSpec{
			Template: core_v1.PodTemplateSpec{
				Spec: core_v1.PodSpec{
					Containers: []core_v1.Container{
						core_v1.Container{}, // empty EnvFrom
						core_v1.Container{
							Env: []core_v1.EnvVar{
								core_v1.EnvVar{
									Name: "blah2",
									ValueFrom: &core_v1.EnvVarSource{
										SecretKeyRef: &core_v1.SecretKeySelector{
											Key: "parameters",
											LocalObjectReference: core_v1.LocalObjectReference{
												Name: "secret1",
											},
											Optional: &trueVal,
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

	store := speccheckertesting.FakeStore{Namespace: testNs}
	logger := zaptest.NewLogger(t)
	defer logger.Sync() // nolint: errcheck

	updatedSpec, err := deployment{}.BeforeCreate(&specchecker.Context{Logger: logger, Store: store}, spec)
	require.NoError(t, err)

	deploymentCheck := updatedSpec.(*apps_v1.Deployment)

	require.Contains(t, deploymentCheck.Spec.Template.Annotations, EnvRefHashAnnotation)
	assert.Equal(t, nullSha256, deploymentCheck.Spec.Template.Annotations[EnvRefHashAnnotation])
}

func TestHashNotIgnoredForNonExistingSecret(t *testing.T) {
	t.Parallel()

	deploymentSpec := apps_v1.Deployment{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: apps_v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: testNs,
		},
		Spec: apps_v1.DeploymentSpec{
			Template: core_v1.PodTemplateSpec{
				Spec: core_v1.PodSpec{
					Containers: []core_v1.Container{
						core_v1.Container{}, // empty EnvFrom
						core_v1.Container{
							Env: []core_v1.EnvVar{
								core_v1.EnvVar{
									Name: "blah2",
									ValueFrom: &core_v1.EnvVarSource{
										SecretKeyRef: &core_v1.SecretKeySelector{
											Key: "parameters",
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
		},
	}

	spec := runtimeToUnstructured(t, &deploymentSpec)
	store := speccheckertesting.FakeStore{Namespace: testNs}

	logger := zaptest.NewLogger(t)
	defer logger.Sync() // nolint: errcheck

	_, err := deployment{}.BeforeCreate(&specchecker.Context{Logger: logger, Store: store}, spec)
	require.Error(t, err)
}

func TestNoAnnotationForEmptyDeployment(t *testing.T) {
	t.Parallel()

	deploymentSpec := apps_v1.Deployment{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: apps_v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: testNs,
		},
		Spec: apps_v1.DeploymentSpec{},
	}
	var one int32 = 1
	expectedDeploymentSpec := apps_v1.Deployment{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: apps_v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: testNs,
			Annotations: map[string]string{
				LastAppliedReplicasAnnotation: "1",
			},
		},
		Spec: apps_v1.DeploymentSpec{
			Replicas: &one,
			Template: core_v1.PodTemplateSpec{
				ObjectMeta: meta_v1.ObjectMeta{
					Annotations: map[string]string{
						EnvRefHashAnnotation: nullSha256,
					},
				},
			},
		},
	}

	spec := runtimeToUnstructured(t, &deploymentSpec)
	expectedSpec := runtimeToUnstructured(t, &expectedDeploymentSpec)
	store := speccheckertesting.FakeStore{Namespace: testNs}

	logger := zaptest.NewLogger(t)
	defer logger.Sync() // nolint: errcheck

	updatedSpec, err := deployment{}.BeforeCreate(&specchecker.Context{Logger: logger, Store: store}, spec)
	require.NoError(t, err)

	updatedSpecUnstr := runtimeToUnstructured(t, updatedSpec)

	if !assert.True(t, equality.Semantic.DeepEqual(expectedSpec.Object, updatedSpecUnstr.Object)) {
		t.Log(diff.ObjectReflectDiff(expectedSpec.Object, updatedSpecUnstr.Object))
	}
}

func TestEmptyAnnotationForDeploymentThatDoesntUseAnything(t *testing.T) {
	t.Parallel()

	deploymentSpec := apps_v1.Deployment{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: apps_v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: testNs,
		},
		Spec: apps_v1.DeploymentSpec{
			Template: core_v1.PodTemplateSpec{
				Spec: core_v1.PodSpec{
					Containers: []core_v1.Container{
						core_v1.Container{}, // empty EnvFrom
						core_v1.Container{
							Env: []core_v1.EnvVar{
								core_v1.EnvVar{
									Name:  "blah1",
									Value: "my env var",
								},
							},
						},
						core_v1.Container{
							Env: []core_v1.EnvVar{
								core_v1.EnvVar{
									Name: "blah1",
									ValueFrom: &core_v1.EnvVarSource{
										FieldRef: &core_v1.ObjectFieldSelector{
											APIVersion: "something",
											FieldPath:  "something else",
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

	logger := zaptest.NewLogger(t)
	defer logger.Sync() // nolint: errcheck
	store := speccheckertesting.FakeStore{Namespace: testNs}

	updatedSpec, err := deployment{}.BeforeCreate(&specchecker.Context{Logger: logger, Store: store}, spec)
	require.NoError(t, err)

	deploymentCheck := updatedSpec.(*apps_v1.Deployment)

	require.Contains(t, deploymentCheck.Spec.Template.Annotations, EnvRefHashAnnotation)
	assert.Equal(t, nullSha256, deploymentCheck.Spec.Template.Annotations[EnvRefHashAnnotation])
}

func TestDeploymentAnnotationExplicitlyDisabled(t *testing.T) {
	t.Parallel()

	deploymentSpec := apps_v1.Deployment{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: apps_v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: testNs,
		},
		Spec: apps_v1.DeploymentSpec{
			Template: core_v1.PodTemplateSpec{
				ObjectMeta: meta_v1.ObjectMeta{
					Annotations: map[string]string{
						EnvRefHashAnnotation: "disabled",
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

	logger := zaptest.NewLogger(t)
	defer logger.Sync() // nolint: errcheck
	store := speccheckertesting.FakeStore{Namespace: testNs}

	updatedSpec, err := deployment{}.BeforeCreate(&specchecker.Context{Logger: logger, Store: store}, spec)
	require.NoError(t, err)

	deploymentCheck := updatedSpec.(*apps_v1.Deployment)

	assert.Contains(t, deploymentCheck.Spec.Template.Annotations, EnvRefHashAnnotation)
	assert.Contains(t, deploymentCheck.Spec.Template.Annotations, EnvRefHashAnnotation)
	assert.Equal(t, deploymentCheck.Spec.Template.Annotations[EnvRefHashAnnotation], "disabled")
}

func TestUserEnteredAnnotationOverridden(t *testing.T) {
	t.Parallel()

	deploymentSpec := apps_v1.Deployment{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: apps_v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: testNs,
		},
		Spec: apps_v1.DeploymentSpec{
			Template: core_v1.PodTemplateSpec{
				ObjectMeta: meta_v1.ObjectMeta{
					Annotations: map[string]string{
						EnvRefHashAnnotation: "mashingonthekeyboard",
					},
				},
				Spec: core_v1.PodSpec{},
			},
		},
	}

	spec := runtimeToUnstructured(t, &deploymentSpec)

	logger := zaptest.NewLogger(t)
	defer logger.Sync() // nolint: errcheck
	store := speccheckertesting.FakeStore{Namespace: testNs}

	updatedSpec, err := deployment{}.BeforeCreate(&specchecker.Context{Logger: logger, Store: store}, spec)
	require.NoError(t, err)

	deploymentCheck := updatedSpec.(*apps_v1.Deployment)

	assert.Contains(t, deploymentCheck.Spec.Template.Annotations, EnvRefHashAnnotation)
	assert.Equal(t, nullSha256, deploymentCheck.Spec.Template.Annotations[EnvRefHashAnnotation])
}

func TestUserEnteredAnnotationInDeploymentWithRefs(t *testing.T) {
	t.Parallel()

	expectedAnnotationValue := "mashingonthekeyboard"
	deploymentSpec := apps_v1.Deployment{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: apps_v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: testNs,
		},
		Spec: apps_v1.DeploymentSpec{
			Template: core_v1.PodTemplateSpec{
				ObjectMeta: meta_v1.ObjectMeta{
					Annotations: map[string]string{
						EnvRefHashAnnotation: expectedAnnotationValue,
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

	store := speccheckertesting.FakeStore{
		Namespace: testNs,
		Responses: map[string]runtime.Object{
			"secret1": &core_v1.Secret{
				TypeMeta: meta_v1.TypeMeta{
					Kind:       "Secret",
					APIVersion: "v1",
				},
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "secret1",
					Namespace: testNs,
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
	}

	logger := zaptest.NewLogger(t)
	defer logger.Sync() // nolint: errcheck

	updatedSpec, err := deployment{}.BeforeCreate(&specchecker.Context{Logger: logger, Store: store}, spec)
	require.NoError(t, err)

	deploymentCheck := updatedSpec.(*apps_v1.Deployment)
	assert.Contains(t, deploymentCheck.Spec.Template.Annotations, EnvRefHashAnnotation)

	// The annotation is no longer equal
	assert.NotEqual(t, deploymentCheck.Spec.Template.Annotations[EnvRefHashAnnotation], expectedAnnotationValue)
}

func TestDeploymentUpdatedSecrets(t *testing.T) {
	t.Parallel()

	deploymentSpec := apps_v1.Deployment{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: apps_v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: testNs,
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
				Name:      "secret1",
				Namespace: testNs,
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
				Name:      "secret2",
				Namespace: testNs,
			},
			Data: map[string][]byte{
				"parameters": []byte(`{
							"iamRole": "some-role"
							}
						}`),
			},
		},
	}
	store := speccheckertesting.FakeStore{
		Namespace: testNs,
		Responses: allResponses,
	}

	logger := zaptest.NewLogger(t)
	defer logger.Sync() // nolint: errcheck

	updatedSpec, err := deployment{}.BeforeCreate(&specchecker.Context{Logger: logger, Store: store}, spec)
	require.NoError(t, err)

	deploymentCheck := updatedSpec.(*apps_v1.Deployment)
	assert.Contains(t, deploymentCheck.Spec.Template.Annotations, EnvRefHashAnnotation)

	firstHash := deploymentCheck.Spec.Template.Annotations

	allResponses["secret1"] = allResponses["secret2"]
	store = speccheckertesting.FakeStore{
		Namespace: testNs,
		Responses: allResponses,
	}

	updatedSpec, err = deployment{}.BeforeCreate(&specchecker.Context{Logger: logger, Store: store}, spec)
	require.NoError(t, err)

	secondDeploymentCheck := updatedSpec.(*apps_v1.Deployment)
	assert.Contains(t, secondDeploymentCheck.Spec.Template.Annotations, EnvRefHashAnnotation)
	assert.NotEqual(t, secondDeploymentCheck.Spec.Template.Annotations[EnvRefHashAnnotation], firstHash)
}

func TestLastAppliedReplicasExplicitlyDisabled(t *testing.T) {
	t.Parallel()

	deploymentSpec := apps_v1.Deployment{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: apps_v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: testNs,
			Annotations: map[string]string{
				LastAppliedReplicasAnnotation: "disabled",
			},
		},
		Spec: apps_v1.DeploymentSpec{
			Template: core_v1.PodTemplateSpec{
				Spec: core_v1.PodSpec{
					Containers: []core_v1.Container{
						core_v1.Container{
							Image: "some/image:tag",
						},
					},
				},
			},
		},
	}

	spec := runtimeToUnstructured(t, &deploymentSpec)

	logger := zaptest.NewLogger(t)
	defer logger.Sync() // nolint: errcheck
	store := speccheckertesting.FakeStore{Namespace: testNs}

	updatedSpec, err := deployment{}.BeforeCreate(&specchecker.Context{Logger: logger, Store: store}, spec)
	require.NoError(t, err)

	deploymentCheck := updatedSpec.(*apps_v1.Deployment)
	assert.Contains(t, deploymentCheck.Annotations, LastAppliedReplicasAnnotation)
	assert.Equal(t, "disabled", deploymentCheck.Annotations[LastAppliedReplicasAnnotation])
}

func runtimeToUnstructured(t *testing.T, obj runtime.Object) *unstructured.Unstructured {
	out, err := util.RuntimeToUnstructured(obj)
	require.NoError(t, err)
	return out
}
