package builtin

import (
	"testing"

	"github.com/atlassian/smith/pkg/speccheck"
	specchecktesting "github.com/atlassian/smith/pkg/speccheck/testing"
	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	core_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestSameChecksumIfNoChanges(t *testing.T) {
	t.Parallel()

	instanceSpec := sc_v1b1.ServiceInstance{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "ServiceInstance",
			APIVersion: sc_v1b1.SchemeGroupVersion.String(),
		},
		Spec: sc_v1b1.ServiceInstanceSpec{
			ParametersFrom: []sc_v1b1.ParametersFromSource{
				{
					SecretKeyRef: &sc_v1b1.SecretKeyReference{
						Name: "secret1",
						Key:  "parameters",
					},
				},
				{
					SecretKeyRef: &sc_v1b1.SecretKeyReference{
						Name: "secret2",
						Key:  "parameters",
					},
				},
			},
		},
	}

	spec := runtimeToUnstructured(t, &instanceSpec)

	store := specchecktesting.FakeStore{
		Responses: map[string]runtime.Object{
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
		},
	}
	logger := zaptest.NewLogger(t)
	defer logger.Sync() // nolint: errcheck

	ctx := &speccheck.Context{Logger: logger, Store: store}
	updatedSpec, err := serviceInstance{}.BeforeCreate(ctx, spec)
	require.NoError(t, err)

	instanceCheck := updatedSpec.(*sc_v1b1.ServiceInstance)

	assert.Contains(t, instanceCheck.Annotations, SecretParametersChecksumAnnotation)
	assert.Zero(t, instanceCheck.Spec.UpdateRequests, "expected UpdateRequests to be 0 for create")
	firstCheckSum := instanceCheck.Annotations[SecretParametersChecksumAnnotation]

	updateTwice, err := serviceInstance{}.ApplySpec(ctx, spec, runtimeToUnstructured(t, instanceCheck))
	require.NoError(t, err)
	secondInstance := updateTwice.(*sc_v1b1.ServiceInstance)

	assert.Contains(t, secondInstance.Annotations, SecretParametersChecksumAnnotation)
	assert.Zero(t, secondInstance.Spec.UpdateRequests, "expected UpdateRequests to be 0 for create")
	assert.Equal(t, firstCheckSum, secondInstance.Annotations[SecretParametersChecksumAnnotation])
}

func TestAnnotationAddedForEmptyParametersFrom(t *testing.T) {
	t.Parallel()

	instanceSpec := sc_v1b1.ServiceInstance{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "ServiceInstance",
			APIVersion: sc_v1b1.SchemeGroupVersion.String(),
		},
		Spec: sc_v1b1.ServiceInstanceSpec{},
	}

	spec := runtimeToUnstructured(t, &instanceSpec)

	logger := zaptest.NewLogger(t)
	defer logger.Sync() // nolint: errcheck
	ctx := &speccheck.Context{Logger: logger, Store: specchecktesting.FakeStore{}}
	updatedSpec, err := serviceInstance{}.BeforeCreate(ctx, spec)
	require.NoError(t, err)

	instanceCheck := updatedSpec.(*sc_v1b1.ServiceInstance)

	assert.Contains(t, instanceCheck.Annotations, SecretParametersChecksumAnnotation)
}

func TestExplicitlyDisabled(t *testing.T) {
	t.Parallel()

	instanceSpec := sc_v1b1.ServiceInstance{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "ServiceInstance",
			APIVersion: sc_v1b1.SchemeGroupVersion.String(),
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Annotations: map[string]string{
				SecretParametersChecksumAnnotation: "disabled",
			},
		},
		Spec: sc_v1b1.ServiceInstanceSpec{
			ParametersFrom: []sc_v1b1.ParametersFromSource{
				{
					SecretKeyRef: &sc_v1b1.SecretKeyReference{
						Name: "secret1",
						Key:  "parameters",
					},
				},
			},
		},
	}

	spec := runtimeToUnstructured(t, &instanceSpec)

	logger := zaptest.NewLogger(t)
	defer logger.Sync() // nolint: errcheck
	ctx := &speccheck.Context{Logger: logger, Store: specchecktesting.FakeStore{}}
	updatedSpec, err := serviceInstance{}.BeforeCreate(ctx, spec)
	require.NoError(t, err)

	instanceCheck := updatedSpec.(*sc_v1b1.ServiceInstance)

	assert.Contains(t, instanceCheck.Annotations, SecretParametersChecksumAnnotation)
	assert.Equal(t, instanceCheck.Annotations[SecretParametersChecksumAnnotation], "disabled")
	assert.Zero(t, instanceCheck.Spec.UpdateRequests, "expected UpdateRequests to be 0 for create")
}

func TestUpdateInstanceSecrets(t *testing.T) {
	t.Parallel()

	instanceSpec := sc_v1b1.ServiceInstance{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "ServiceInstance",
			APIVersion: sc_v1b1.SchemeGroupVersion.String(),
		},
		Spec: sc_v1b1.ServiceInstanceSpec{
			ParametersFrom: []sc_v1b1.ParametersFromSource{
				{
					SecretKeyRef: &sc_v1b1.SecretKeyReference{
						Name: "secret1",
						Key:  "parameters",
					},
				},
				{
					SecretKeyRef: &sc_v1b1.SecretKeyReference{
						Name: "secret2",
						Key:  "parameters",
					},
				},
			},
		},
	}

	spec := runtimeToUnstructured(t, &instanceSpec)

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
	logger := zaptest.NewLogger(t)
	defer logger.Sync() // nolint: errcheck
	ctx := &speccheck.Context{Logger: logger, Store: specchecktesting.FakeStore{
		Responses: allResponses,
	}}
	updatedSpec, err := serviceInstance{}.BeforeCreate(ctx, spec)
	require.NoError(t, err)

	instanceCheck := updatedSpec.(*sc_v1b1.ServiceInstance)

	assert.Contains(t, instanceCheck.Annotations, SecretParametersChecksumAnnotation)
	assert.Zero(t, instanceCheck.Spec.UpdateRequests, "expected UpdateRequests to be 0 for create")
	firstCheckSum := instanceCheck.Annotations[SecretParametersChecksumAnnotation]

	allResponses["secret1"] = allResponses["secret2"]

	ctx = &speccheck.Context{Logger: logger, Store: specchecktesting.FakeStore{
		Responses: allResponses,
	}}
	updateTwice, err := serviceInstance{}.ApplySpec(ctx, spec, runtimeToUnstructured(t, instanceCheck))
	require.NoError(t, err)
	secondInstance := updateTwice.(*sc_v1b1.ServiceInstance)

	assert.Contains(t, secondInstance.Annotations, SecretParametersChecksumAnnotation)
	assert.EqualValues(t, 1, secondInstance.Spec.UpdateRequests, "expected UpdateRequests to be 1 for updated data")
	assert.NotEqual(t, firstCheckSum, secondInstance.Annotations[SecretParametersChecksumAnnotation])
}

func TestUserEnteredAnnotationNoRefs(t *testing.T) {
	t.Parallel()

	userAnnotationValue := "mashingonthekeyboard"
	instanceSpec := sc_v1b1.ServiceInstance{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "ServiceInstance",
			APIVersion: sc_v1b1.SchemeGroupVersion.String(),
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Annotations: map[string]string{
				SecretParametersChecksumAnnotation: userAnnotationValue,
			},
		},
		Spec: sc_v1b1.ServiceInstanceSpec{},
	}
	spec := runtimeToUnstructured(t, &instanceSpec)

	logger := zaptest.NewLogger(t)
	defer logger.Sync() // nolint: errcheck
	updatedSpec, err := serviceInstance{}.BeforeCreate(&speccheck.Context{Logger: logger, Store: specchecktesting.FakeStore{}}, spec)
	require.NoError(t, err)

	instanceCheck := updatedSpec.(*sc_v1b1.ServiceInstance)

	assert.Contains(t, instanceCheck.Annotations, SecretParametersChecksumAnnotation)
	assert.Zero(t, instanceCheck.Spec.UpdateRequests, "expected UpdateRequests to be 0 when overriding user the first time")
	firstAnnotationValue := instanceCheck.Annotations[SecretParametersChecksumAnnotation]
	assert.NotEqual(t, firstAnnotationValue, userAnnotationValue)

	compareToPreviousUpdate, err := serviceInstance{}.ApplySpec(&speccheck.Context{Logger: logger, Store: specchecktesting.FakeStore{}}, spec, runtimeToUnstructured(t, instanceCheck))
	require.NoError(t, err)

	ignoreUserValue := compareToPreviousUpdate.(*sc_v1b1.ServiceInstance)

	assert.Contains(t, ignoreUserValue.Annotations, SecretParametersChecksumAnnotation)
	assert.Zero(t, ignoreUserValue.Spec.UpdateRequests, "expected UpdateRequests to be 0 when overriding user the first time")
	assert.Equal(t, ignoreUserValue.Annotations[SecretParametersChecksumAnnotation], firstAnnotationValue)
}

func TestUserEnteredAnnotationWithRefs(t *testing.T) {
	t.Parallel()

	userAnnotationValue := "copy+pasted something"

	instanceSpec := sc_v1b1.ServiceInstance{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "ServiceInstance",
			APIVersion: sc_v1b1.SchemeGroupVersion.String(),
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Annotations: map[string]string{
				SecretParametersChecksumAnnotation: userAnnotationValue,
			},
		},
		Spec: sc_v1b1.ServiceInstanceSpec{
			ParametersFrom: []sc_v1b1.ParametersFromSource{
				{
					SecretKeyRef: &sc_v1b1.SecretKeyReference{
						Name: "secret1",
						Key:  "parameters",
					},
				},
			},
		},
	}

	spec := runtimeToUnstructured(t, &instanceSpec)

	store := specchecktesting.FakeStore{
		Responses: map[string]runtime.Object{
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
	}

	logger := zaptest.NewLogger(t)
	defer logger.Sync() // nolint: errcheck
	updatedSpec, err := serviceInstance{}.BeforeCreate(&speccheck.Context{Logger: logger, Store: store}, spec)
	require.NoError(t, err)

	instanceCheck := updatedSpec.(*sc_v1b1.ServiceInstance)

	assert.Contains(t, instanceCheck.Annotations, SecretParametersChecksumAnnotation)
	assert.Zero(t, instanceCheck.Spec.UpdateRequests, "expected UpdateRequests to be 0 when overriding user the first time")
	firstAnnotationValue := instanceCheck.Annotations[SecretParametersChecksumAnnotation]
	assert.NotEqual(t, firstAnnotationValue, userAnnotationValue)

	compareToPreviousUpdate, err := serviceInstance{}.ApplySpec(&speccheck.Context{Logger: logger, Store: store}, spec, runtimeToUnstructured(t, instanceCheck))
	require.NoError(t, err)

	ignoreUserValue := compareToPreviousUpdate.(*sc_v1b1.ServiceInstance)

	assert.Contains(t, ignoreUserValue.Annotations, SecretParametersChecksumAnnotation)
	assert.Zero(t, ignoreUserValue.Spec.UpdateRequests, "expected UpdateRequests to be 0 when overriding user the first time")
	assert.Equal(t, ignoreUserValue.Annotations[SecretParametersChecksumAnnotation], firstAnnotationValue)
}
