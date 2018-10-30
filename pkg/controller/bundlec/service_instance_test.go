package bundlec

import (
	"testing"

	"github.com/atlassian/smith"
	"github.com/atlassian/smith/pkg/util"
	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apps_v1 "k8s.io/api/apps/v1"
	core_v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"
)

const (
	siTestNamespace     = "ns"
	siTestAnnotationKey = smith.Domain + "/secretParametersChecksum"
)

type fakeStore struct {
	responses map[string]runtime.Object
}

func (f fakeStore) Get(gvk schema.GroupVersionKind, namespace, name string) (obj runtime.Object, exists bool, err error) {
	v, ok := f.responses[name]
	return v, ok, nil
}

func (f fakeStore) ObjectsControlledBy(namespace string, uid types.UID) ([]runtime.Object, error) {
	return nil, nil
}

func (f fakeStore) AddInformer(schema.GroupVersionKind, cache.SharedIndexInformer) error {
	return nil
}

func (f fakeStore) RemoveInformer(schema.GroupVersionKind) bool {
	return false
}

func serviceInstanceUnmarshal(t *testing.T, spec *unstructured.Unstructured) *sc_v1b1.ServiceInstance {
	var instanceSpec sc_v1b1.ServiceInstance
	err := util.ConvertType(scheme(t), spec, &instanceSpec)
	require.NoError(t, err)
	return &instanceSpec
}

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
			},
		},
		scheme: scheme(t),
	}

	updatedSpec, err := rst.forceServiceInstanceUpdates(spec, nil, siTestNamespace)
	require.NoError(t, err)

	instanceCheck := serviceInstanceUnmarshal(t, updatedSpec)

	assert.Contains(t, instanceCheck.Annotations, siTestAnnotationKey)
	assert.Zero(t, instanceCheck.Spec.UpdateRequests, "expected UpdateRequests to be 0 for create")
	firstCheckSum := instanceCheck.ObjectMeta.Annotations[siTestAnnotationKey]

	updateTwice, err := rst.forceServiceInstanceUpdates(spec, instanceCheck, siTestNamespace)
	require.NoError(t, err)
	secondInstance := serviceInstanceUnmarshal(t, updateTwice)

	assert.Contains(t, secondInstance.Annotations, smith.Domain+"/secretParametersChecksum")
	assert.Zero(t, secondInstance.Spec.UpdateRequests, "expected UpdateRequests to be 0 for create")
	assert.Equal(t, firstCheckSum, secondInstance.ObjectMeta.Annotations[siTestAnnotationKey])
}

func TestNoAnnotationForEmptyParameretersFrom(t *testing.T) {
	t.Parallel()

	instanceSpec := sc_v1b1.ServiceInstance{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "ServiceInstance",
			APIVersion: sc_v1b1.SchemeGroupVersion.String(),
		},
		Spec: sc_v1b1.ServiceInstanceSpec{},
	}

	spec := runtimeToUnstructured(t, &instanceSpec)

	rst := resourceSyncTask{
		scheme: scheme(t),
	}

	updatedSpec, err := rst.forceServiceInstanceUpdates(spec, nil, siTestNamespace)
	require.NoError(t, err)

	assert.True(t, equality.Semantic.DeepEqual(spec.Object, updatedSpec.Object))
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
				siTestAnnotationKey: "disabled",
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

	rst := resourceSyncTask{
		scheme: scheme(t),
	}

	updatedSpec, err := rst.forceServiceInstanceUpdates(spec, nil, siTestNamespace)
	require.NoError(t, err)

	instanceCheck := serviceInstanceUnmarshal(t, updatedSpec)

	assert.Contains(t, instanceCheck.Annotations, siTestAnnotationKey)
	assert.Equal(t, instanceCheck.Annotations[siTestAnnotationKey], "disabled")
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

	rst := resourceSyncTask{
		store: fakeStore{
			responses: allResponses,
		},
		scheme: scheme(t),
	}

	updatedSpec, err := rst.forceServiceInstanceUpdates(spec, nil, siTestNamespace)
	require.NoError(t, err)

	instanceCheck := serviceInstanceUnmarshal(t, updatedSpec)

	assert.Contains(t, instanceCheck.Annotations, siTestAnnotationKey)
	assert.Zero(t, instanceCheck.Spec.UpdateRequests, "expected UpdateRequests to be 0 for create")
	firstCheckSum := instanceCheck.ObjectMeta.Annotations[siTestAnnotationKey]

	allResponses["secret1"] = allResponses["secret2"]

	rstUpdatedMocks := resourceSyncTask{
		store: fakeStore{
			responses: allResponses,
		},
		scheme: scheme(t),
	}

	updateTwice, err := rstUpdatedMocks.forceServiceInstanceUpdates(spec, instanceCheck, siTestNamespace)
	require.NoError(t, err)
	secondInstance := serviceInstanceUnmarshal(t, updateTwice)

	assert.Contains(t, secondInstance.Annotations, smith.Domain+"/secretParametersChecksum")
	assert.True(t, secondInstance.Spec.UpdateRequests == 1, "expected UpdateRequests to be 1 for updated data")
	assert.NotEqual(t, firstCheckSum, secondInstance.ObjectMeta.Annotations[siTestAnnotationKey])
}

func TestUserEnteredAnnotationNoRefs(t *testing.T) {
	t.Parallel()

	expectedAnnotationValue := "mashingonthekeyboard"
	instanceSpec := sc_v1b1.ServiceInstance{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "ServiceInstance",
			APIVersion: sc_v1b1.SchemeGroupVersion.String(),
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Annotations: map[string]string{
				siTestAnnotationKey: expectedAnnotationValue,
			},
		},
		Spec: sc_v1b1.ServiceInstanceSpec{},
	}

	spec := runtimeToUnstructured(t, &instanceSpec)

	rst := resourceSyncTask{
		scheme: scheme(t),
	}

	updatedSpec, err := rst.forceServiceInstanceUpdates(spec, nil, siTestNamespace)
	require.NoError(t, err)

	instanceCheck := serviceInstanceUnmarshal(t, updatedSpec)

	assert.Contains(t, instanceCheck.Annotations, siTestAnnotationKey)
	assert.Zero(t, instanceCheck.Spec.UpdateRequests, "expected UpdateRequests to be 0 for create")
	assert.Equal(t, instanceCheck.ObjectMeta.Annotations[siTestAnnotationKey], expectedAnnotationValue)
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
				siTestAnnotationKey: userAnnotationValue,
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

	updatedSpec, err := rst.forceServiceInstanceUpdates(spec, nil, siTestNamespace)
	require.NoError(t, err)

	instanceCheck := serviceInstanceUnmarshal(t, updatedSpec)

	assert.Contains(t, instanceCheck.Annotations, siTestAnnotationKey)
	assert.Zero(t, instanceCheck.Spec.UpdateRequests, "expected UpdateRequests to be 0 when overriding user the first time")
	assert.NotEqual(t, instanceCheck.ObjectMeta.Annotations[siTestAnnotationKey], userAnnotationValue)
	firstAnnotationValue := instanceCheck.ObjectMeta.Annotations[siTestAnnotationKey]

	compareToPreviousUpdate, err := rst.forceServiceInstanceUpdates(spec, instanceCheck, siTestNamespace)
	require.NoError(t, err)

	ignoreUserValue := serviceInstanceUnmarshal(t, compareToPreviousUpdate)

	assert.Contains(t, ignoreUserValue.Annotations, siTestAnnotationKey)
	assert.Zero(t, ignoreUserValue.Spec.UpdateRequests, "expected UpdateRequests to be 0 when overriding user the first time")
	assert.NotEqual(t, ignoreUserValue.ObjectMeta.Annotations[siTestAnnotationKey], userAnnotationValue)
	assert.Equal(t, ignoreUserValue.ObjectMeta.Annotations[siTestAnnotationKey], firstAnnotationValue)
}

func scheme(t *testing.T) *runtime.Scheme {
	scheme := runtime.NewScheme()
	require.NoError(t, core_v1.AddToScheme(scheme))
	require.NoError(t, sc_v1b1.AddToScheme(scheme))
	require.NoError(t, apps_v1.AddToScheme(scheme))
	return scheme
}

func runtimeToUnstructured(t *testing.T, obj runtime.Object) *unstructured.Unstructured {
	out, err := util.RuntimeToUnstructured(obj)
	require.NoError(t, err)
	return out
}
