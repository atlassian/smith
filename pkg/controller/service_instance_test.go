package controller

import (
	"testing"

	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"fmt"

	"github.com/atlassian/smith/pkg/util"
	"github.com/stretchr/testify/assert"
	core_v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/atlassian/smith"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	unstructured_conversion "k8s.io/apimachinery/pkg/conversion/unstructured"
)

const (
	defaultNamespace = "ns"
	annotaionKey     = smith.Domain + "/secretParametersChecksum"
)

type fakeStore struct {
	responses map[string]runtime.Object
}

func (f fakeStore) Get(gvk schema.GroupVersionKind, namespace, name string) (obj runtime.Object, exists bool, err error) {
	v, ok := f.responses[name]
	if !ok {
		panic(fmt.Sprintf("Could not find expected response in mock object for key %q", name))
	}
	return v, ok, nil
}
func (f fakeStore) GetObjectsForBundle(namespace, bundleName string) ([]runtime.Object, error) {
	return nil, nil
}
func (f fakeStore) AddInformer(schema.GroupVersionKind, cache.SharedIndexInformer) {}
func (f fakeStore) RemoveInformer(schema.GroupVersionKind) bool {
	return false
}

func serviceInstance(spec *unstructured.Unstructured) *sc_v1b1.ServiceInstance {
	var instanceSpec sc_v1b1.ServiceInstance
	if err := unstructured_conversion.DefaultConverter.FromUnstructured(spec.Object, &instanceSpec); err != nil {
		panic(err)
	}
	return &instanceSpec
}

func TestSameChecksumIfNoChanges(t *testing.T) {
	t.Parallel()

	instanceSpec := sc_v1b1.ServiceInstance{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "ServiceInstance",
			APIVersion: sc_v1b1.GroupName + "/v1",
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

	spec, err := util.RuntimeToUnstructured(&instanceSpec)
	require.NoError(t, err)

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
	}

	updatedSpec, err := rst.forceServiceInstanceUpdates(spec, nil, defaultNamespace)
	require.NoError(t, err)

	instanceCheck := serviceInstance(updatedSpec)

	assert.Contains(t, instanceCheck.Annotations, annotaionKey)
	assert.True(t, instanceCheck.Spec.UpdateRequests == 0, "expected UpdateRequests to be 0 for create")
	firstCheckSum := instanceCheck.ObjectMeta.Annotations[annotaionKey]

	updateTwice, err := rst.forceServiceInstanceUpdates(spec, instanceCheck, defaultNamespace)
	require.NoError(t, err)
	secondInstance := serviceInstance(updateTwice)

	assert.Contains(t, secondInstance.Annotations, smith.Domain+"/secretParametersChecksum")
	assert.True(t, secondInstance.Spec.UpdateRequests == 0, "expected UpdateRequests to be 0 for create")
	assert.Equal(t, firstCheckSum, secondInstance.ObjectMeta.Annotations[annotaionKey])
}

func TestNoAnnotationForEmptyParameretersFrom(t *testing.T) {
	t.Parallel()

	instanceSpec := sc_v1b1.ServiceInstance{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "ServiceInstance",
			APIVersion: sc_v1b1.GroupName + "/v1",
		},
		Spec: sc_v1b1.ServiceInstanceSpec{},
	}

	spec, err := util.RuntimeToUnstructured(&instanceSpec)
	require.NoError(t, err)

	rst := resourceSyncTask{}

	updatedSpec, err := rst.forceServiceInstanceUpdates(spec, nil, defaultNamespace)
	require.NoError(t, err)

	assert.True(t, equality.Semantic.DeepEqual(spec.Object, updatedSpec.Object))
}

func TestExplicitlyDisabled(t *testing.T) {
	t.Parallel()

	instanceSpec := sc_v1b1.ServiceInstance{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "ServiceInstance",
			APIVersion: sc_v1b1.GroupName + "/v1",
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Annotations: map[string]string{
				annotaionKey: "disabled",
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

	spec, err := util.RuntimeToUnstructured(&instanceSpec)
	require.NoError(t, err)

	rst := resourceSyncTask{}

	updatedSpec, err := rst.forceServiceInstanceUpdates(spec, nil, defaultNamespace)
	require.NoError(t, err)

	instanceCheck := serviceInstance(updatedSpec)

	assert.Contains(t, instanceCheck.Annotations, annotaionKey)
	assert.Equal(t, instanceCheck.Annotations[annotaionKey], "disabled")
	assert.True(t, instanceCheck.Spec.UpdateRequests == 0, "expected UpdateRequests to be 0 for create")
}

func TestUpdateInstanceSecrets(t *testing.T) {
	instanceSpec := sc_v1b1.ServiceInstance{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "ServiceInstance",
			APIVersion: sc_v1b1.GroupName + "/v1",
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

	spec, err := util.RuntimeToUnstructured(&instanceSpec)
	require.NoError(t, err)

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
	}

	updatedSpec, err := rst.forceServiceInstanceUpdates(spec, nil, defaultNamespace)
	require.NoError(t, err)

	instanceCheck := serviceInstance(updatedSpec)

	assert.Contains(t, instanceCheck.Annotations, annotaionKey)
	assert.True(t, instanceCheck.Spec.UpdateRequests == 0, "expected UpdateRequests to be 0 for create")
	firstCheckSum := instanceCheck.ObjectMeta.Annotations[annotaionKey]

	allResponses["secret1"] = allResponses["secret2"]

	rstUpdatedMocks := resourceSyncTask{
		store: fakeStore{
			responses: allResponses,
		},
	}

	updateTwice, err := rstUpdatedMocks.forceServiceInstanceUpdates(spec, instanceCheck, defaultNamespace)
	require.NoError(t, err)
	secondInstance := serviceInstance(updateTwice)

	assert.Contains(t, secondInstance.Annotations, smith.Domain+"/secretParametersChecksum")
	assert.True(t, secondInstance.Spec.UpdateRequests == 1, "expected UpdateRequests to be 1 for updated data")
	assert.NotEqual(t, firstCheckSum, secondInstance.ObjectMeta.Annotations[annotaionKey])
}

func TestUserEnteredAnnotationNoRefs(t *testing.T) {
	t.Parallel()
	expectedAnnotationValue := "mashingonthekeyboard"
	instanceSpec := sc_v1b1.ServiceInstance{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "ServiceInstance",
			APIVersion: sc_v1b1.GroupName + "/v1",
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Annotations: map[string]string{
				annotaionKey: expectedAnnotationValue,
			},
		},
		Spec: sc_v1b1.ServiceInstanceSpec{},
	}

	spec, err := util.RuntimeToUnstructured(&instanceSpec)
	require.NoError(t, err)

	rst := resourceSyncTask{}

	updatedSpec, err := rst.forceServiceInstanceUpdates(spec, nil, defaultNamespace)
	require.NoError(t, err)

	instanceCheck := serviceInstance(updatedSpec)

	assert.Contains(t, instanceCheck.Annotations, annotaionKey)
	assert.True(t, instanceCheck.Spec.UpdateRequests == 0, "expected UpdateRequests to be 0 for create")
	assert.Equal(t, instanceCheck.ObjectMeta.Annotations[annotaionKey], expectedAnnotationValue)
}

func TestUserEnteredAnnotationWithRefs(t *testing.T) {
	t.Parallel()
	userAnnotationValue := "copy+pasted something"

	instanceSpec := sc_v1b1.ServiceInstance{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "ServiceInstance",
			APIVersion: sc_v1b1.GroupName + "/v1",
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Annotations: map[string]string{
				annotaionKey: userAnnotationValue,
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

	spec, err := util.RuntimeToUnstructured(&instanceSpec)
	require.NoError(t, err)

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
	}

	updatedSpec, err := rst.forceServiceInstanceUpdates(spec, nil, defaultNamespace)
	require.NoError(t, err)

	instanceCheck := serviceInstance(updatedSpec)

	assert.Contains(t, instanceCheck.Annotations, annotaionKey)
	assert.True(t, instanceCheck.Spec.UpdateRequests == 0, "expected UpdateRequests to be 0 when overriding user the first time")
	assert.NotEqual(t, instanceCheck.ObjectMeta.Annotations[annotaionKey], userAnnotationValue)
	firstAnnotationValue := instanceCheck.ObjectMeta.Annotations[annotaionKey]

	compareToPreviousUpdate, err := rst.forceServiceInstanceUpdates(spec, instanceCheck, defaultNamespace)

	ignoreUserValue := serviceInstance(compareToPreviousUpdate)

	assert.Contains(t, ignoreUserValue.Annotations, annotaionKey)
	assert.True(t, ignoreUserValue.Spec.UpdateRequests == 0, "expected UpdateRequests to be 0 when overriding user the first time")
	assert.NotEqual(t, ignoreUserValue.ObjectMeta.Annotations[annotaionKey], userAnnotationValue)
	assert.Equal(t, ignoreUserValue.ObjectMeta.Annotations[annotaionKey], firstAnnotationValue)
}
