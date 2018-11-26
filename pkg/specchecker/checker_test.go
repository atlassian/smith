package specchecker_test

import (
	"encoding/json"
	"fmt"
	"strconv"
	"testing"

	"github.com/atlassian/smith/pkg/specchecker"
	"github.com/atlassian/smith/pkg/specchecker/builtin"
	speccheckertesting "github.com/atlassian/smith/pkg/specchecker/testing"
	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	apps_v1 "k8s.io/api/apps/v1"
	core_v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestEqualityCheck(t *testing.T) {
	t.Parallel()

	var numberOfReplicas int32 = 3
	var runningNumberOfReplicas int32 = 5
	inputs := []struct {
		name   string
		spec   runtime.Object
		actual runtime.Object
	}{
		{
			name: "Deployment",
			spec: &apps_v1.Deployment{
				TypeMeta: meta_v1.TypeMeta{
					Kind:       "Deployment",
					APIVersion: apps_v1.SchemeGroupVersion.String(),
				},
				Spec: apps_v1.DeploymentSpec{
					Replicas: &numberOfReplicas,
					Template: core_v1.PodTemplateSpec{
						Spec: core_v1.PodSpec{
							Containers: []core_v1.Container{
								{
									Name:  "c1",
									Image: "ima.ge",
									Ports: []core_v1.ContainerPort{
										{
											Name:          "http",
											ContainerPort: 8080,
											Protocol:      core_v1.ProtocolTCP,
										},
									},
								},
							},
							ServiceAccountName: "abc",
						},
					},
				},
			},
			actual: &apps_v1.Deployment{
				TypeMeta: meta_v1.TypeMeta{
					Kind:       "Deployment",
					APIVersion: apps_v1.SchemeGroupVersion.String(),
				},
				ObjectMeta: meta_v1.ObjectMeta{
					Annotations: map[string]string{
						builtin.LastAppliedReplicasAnnotation: strconv.Itoa(int(numberOfReplicas)),
					},
				},
				Spec: apps_v1.DeploymentSpec{
					Replicas: &numberOfReplicas,
					Template: core_v1.PodTemplateSpec{
						ObjectMeta: meta_v1.ObjectMeta{
							Annotations: map[string]string{
								builtin.EnvRefHashAnnotation: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
							},
						},
						Spec: core_v1.PodSpec{
							Containers: []core_v1.Container{
								{
									Name:  "c1",
									Image: "ima.ge",
									Ports: []core_v1.ContainerPort{
										{
											Name:          "http",
											ContainerPort: 8080,
											Protocol:      core_v1.ProtocolTCP,
										},
									},
								},
							},
							ServiceAccountName:       "abc",
							DeprecatedServiceAccount: "abc",
						},
					},
				},
			},
		},
		{
			name: "Deployment with running replicas changed",
			spec: &apps_v1.Deployment{
				TypeMeta: meta_v1.TypeMeta{
					Kind:       "Deployment",
					APIVersion: apps_v1.SchemeGroupVersion.String(),
				},
				ObjectMeta: meta_v1.ObjectMeta{
					Annotations: map[string]string{
						builtin.LastAppliedReplicasAnnotation: strconv.Itoa(int(numberOfReplicas)),
					},
				},
				Spec: apps_v1.DeploymentSpec{
					Replicas: &numberOfReplicas,
					Template: core_v1.PodTemplateSpec{
						Spec: core_v1.PodSpec{
							Containers: []core_v1.Container{
								{
									Name:  "c1",
									Image: "ima.ge",
									Ports: []core_v1.ContainerPort{
										{
											Name:          "http",
											ContainerPort: 8080,
											Protocol:      core_v1.ProtocolTCP,
										},
									},
								},
							},
							ServiceAccountName: "abc",
						},
					},
				},
			},
			actual: &apps_v1.Deployment{
				TypeMeta: meta_v1.TypeMeta{
					Kind:       "Deployment",
					APIVersion: apps_v1.SchemeGroupVersion.String(),
				},
				ObjectMeta: meta_v1.ObjectMeta{
					Annotations: map[string]string{
						builtin.LastAppliedReplicasAnnotation: strconv.Itoa(int(numberOfReplicas)),
					},
				},
				Spec: apps_v1.DeploymentSpec{
					Replicas: &runningNumberOfReplicas,
					Template: core_v1.PodTemplateSpec{
						ObjectMeta: meta_v1.ObjectMeta{
							Annotations: map[string]string{
								builtin.EnvRefHashAnnotation: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
							},
						},
						Spec: core_v1.PodSpec{
							Containers: []core_v1.Container{
								{
									Name:  "c1",
									Image: "ima.ge",
									Ports: []core_v1.ContainerPort{
										{
											Name:          "http",
											ContainerPort: 8080,
											Protocol:      core_v1.ProtocolTCP,
										},
									},
								},
							},
							ServiceAccountName:       "abc",
							DeprecatedServiceAccount: "abc",
						},
					},
				},
			},
		},
		{
			name: "Service Catalog",
			spec: &sc_v1b1.ServiceInstance{
				TypeMeta: meta_v1.TypeMeta{
					Kind:       "ServiceInstance",
					APIVersion: sc_v1b1.SchemeGroupVersion.String(),
				},
				Spec: sc_v1b1.ServiceInstanceSpec{
					PlanReference: sc_v1b1.PlanReference{
						ClusterServiceClassExternalName: "ClusterServiceClassExternalName",
						ClusterServicePlanExternalName:  "ClusterServicePlanExternalName",
					},
				},
			},
			actual: &sc_v1b1.ServiceInstance{
				TypeMeta: meta_v1.TypeMeta{
					Kind:       "ServiceInstance",
					APIVersion: sc_v1b1.SchemeGroupVersion.String(),
				},
				ObjectMeta: meta_v1.ObjectMeta{
					Annotations: map[string]string{
						builtin.SecretParametersChecksumAnnotation: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
					},
				},
				Spec: sc_v1b1.ServiceInstanceSpec{
					PlanReference: sc_v1b1.PlanReference{
						ClusterServiceClassExternalName: "ClusterServiceClassExternalName",
						ClusterServicePlanExternalName:  "ClusterServicePlanExternalName",
					},
					ClusterServiceClassRef: &sc_v1b1.ClusterObjectReference{
						Name: "ClusterObjectReference",
					},
					ClusterServicePlanRef: &sc_v1b1.ClusterObjectReference{
						Name: "ClusterServicePlanRef",
					},
					ExternalID: "ExternalID",
					UserInfo: &sc_v1b1.UserInfo{
						Username: "Username",
						UID:      "UID",
						Groups:   []string{"group1"},
						Extra: map[string]sc_v1b1.ExtraValue{
							"value1": {"v1"},
						},
					},
					UpdateRequests: 1,
				},
			},
		},
		{
			name: "Service Catalog",
			spec: &sc_v1b1.ServiceInstance{
				TypeMeta: meta_v1.TypeMeta{
					Kind:       "ServiceInstance",
					APIVersion: sc_v1b1.SchemeGroupVersion.String(),
				},
				Spec: sc_v1b1.ServiceInstanceSpec{
					PlanReference: sc_v1b1.PlanReference{
						ServiceClassExternalName: "ServiceClassExternalName",
						ServicePlanExternalName:  "ServicePlanExternalName",
					},
				},
			},
			actual: &sc_v1b1.ServiceInstance{
				TypeMeta: meta_v1.TypeMeta{
					Kind:       "ServiceInstance",
					APIVersion: sc_v1b1.SchemeGroupVersion.String(),
				},
				ObjectMeta: meta_v1.ObjectMeta{
					Annotations: map[string]string{
						builtin.SecretParametersChecksumAnnotation: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
					},
				},
				Spec: sc_v1b1.ServiceInstanceSpec{
					PlanReference: sc_v1b1.PlanReference{
						ServiceClassExternalName: "ServiceClassExternalName",
						ServicePlanExternalName:  "ServicePlanExternalName",
					},
					ClusterServiceClassRef: &sc_v1b1.ClusterObjectReference{
						Name: "ClusterObjectReference",
					},
					ClusterServicePlanRef: &sc_v1b1.ClusterObjectReference{
						Name: "ClusterServicePlanRef",
					},
					ExternalID: "ExternalID",
					UserInfo: &sc_v1b1.UserInfo{
						Username: "Username",
						UID:      "UID",
						Groups:   []string{"group1"},
						Extra: map[string]sc_v1b1.ExtraValue{
							"value1": {"v1"},
						},
					},
					UpdateRequests: 1,
				},
			},
		},
	}
	logger := zaptest.NewLogger(t)
	defer logger.Sync()

	for _, input := range inputs {
		input := input
		t.Run(input.name, func(t *testing.T) {
			t.Parallel()
			sc := specchecker.New(speccheckertesting.FakeStore{}, builtin.ServiceCatalogKnownTypes, builtin.MainKnownTypes)
			_, match, difference, err := sc.CompareActualVsSpec(logger, input.spec, input.actual)
			require.NoError(t, err)
			assert.True(t, match)
			assert.Empty(t, difference)
		})
	}
}

func TestEqualityUnequal(t *testing.T) {
	t.Parallel()

	var numberOfReplicas int32 = 3
	var newNumberOfReplicas int32 = 5
	inputs := []struct {
		name   string
		spec   runtime.Object
		actual runtime.Object
	}{
		{
			name: "Service Catalog with set externalID",
			spec: &sc_v1b1.ServiceInstance{
				TypeMeta: meta_v1.TypeMeta{
					Kind:       "ServiceInstance",
					APIVersion: sc_v1b1.SchemeGroupVersion.String(),
				},
				Spec: sc_v1b1.ServiceInstanceSpec{
					PlanReference: sc_v1b1.PlanReference{
						ClusterServiceClassExternalName: "ClusterServiceClassExternalName",
						ClusterServicePlanExternalName:  "ClusterServicePlanExternalName",
					},
					ExternalID: "foo",
				},
			},
			actual: &sc_v1b1.ServiceInstance{
				TypeMeta: meta_v1.TypeMeta{
					Kind:       "ServiceInstance",
					APIVersion: sc_v1b1.SchemeGroupVersion.String(),
				},
				Spec: sc_v1b1.ServiceInstanceSpec{
					PlanReference: sc_v1b1.PlanReference{
						ClusterServiceClassExternalName: "ClusterServiceClassExternalName",
						ClusterServicePlanExternalName:  "ClusterServicePlanExternalName",
					},
					ClusterServiceClassRef: &sc_v1b1.ClusterObjectReference{
						Name: "ClusterObjectReference",
					},
					ClusterServicePlanRef: &sc_v1b1.ClusterObjectReference{
						Name: "ClusterServicePlanRef",
					},
					ExternalID: "ExternalID",
					UserInfo: &sc_v1b1.UserInfo{
						Username: "Username",
						UID:      "UID",
						Groups:   []string{"group1"},
						Extra: map[string]sc_v1b1.ExtraValue{
							"value1": {"v1"},
						},
					},
					UpdateRequests: 1,
				},
			},
		},
		{
			name: "Deployment with spec replicas changed",
			spec: &apps_v1.Deployment{
				TypeMeta: meta_v1.TypeMeta{
					Kind:       "Deployment",
					APIVersion: apps_v1.SchemeGroupVersion.String(),
				},
				ObjectMeta: meta_v1.ObjectMeta{
					Annotations: map[string]string{builtin.LastAppliedReplicasAnnotation: strconv.Itoa(int(numberOfReplicas))},
				},
				Spec: apps_v1.DeploymentSpec{
					Replicas: &newNumberOfReplicas,
					Template: core_v1.PodTemplateSpec{
						ObjectMeta: meta_v1.ObjectMeta{
							Annotations: map[string]string{
								builtin.EnvRefHashAnnotation: "disabled",
							},
						},
						Spec: core_v1.PodSpec{
							Containers: []core_v1.Container{
								{
									Name:  "c1",
									Image: "ima.ge",
									Ports: []core_v1.ContainerPort{
										{
											Name:          "http",
											ContainerPort: 8080,
											Protocol:      core_v1.ProtocolTCP,
										},
									},
								},
							},
							ServiceAccountName: "abc",
						},
					},
				},
			},
			actual: &apps_v1.Deployment{
				TypeMeta: meta_v1.TypeMeta{
					Kind:       "Deployment",
					APIVersion: apps_v1.SchemeGroupVersion.String(),
				},
				ObjectMeta: meta_v1.ObjectMeta{
					Annotations: map[string]string{builtin.LastAppliedReplicasAnnotation: strconv.Itoa(int(numberOfReplicas))},
				},
				Spec: apps_v1.DeploymentSpec{
					Replicas: &numberOfReplicas,
					Template: core_v1.PodTemplateSpec{
						ObjectMeta: meta_v1.ObjectMeta{
							Annotations: map[string]string{
								builtin.EnvRefHashAnnotation: "disabled",
							},
						},
						Spec: core_v1.PodSpec{
							Containers: []core_v1.Container{
								{
									Name:  "c1",
									Image: "ima.ge",
									Ports: []core_v1.ContainerPort{
										{
											Name:          "http",
											ContainerPort: 8080,
											Protocol:      core_v1.ProtocolTCP,
										},
									},
								},
							},
							ServiceAccountName:       "abc",
							DeprecatedServiceAccount: "abc",
						},
					},
				},
			},
		},
		{
			name: "Deployment with lastAppliedReplicas newly disabled",
			spec: &apps_v1.Deployment{
				TypeMeta: meta_v1.TypeMeta{
					Kind:       "Deployment",
					APIVersion: apps_v1.SchemeGroupVersion.String(),
				},
				ObjectMeta: meta_v1.ObjectMeta{
					Annotations: map[string]string{
						builtin.LastAppliedReplicasAnnotation: "disabled",
					},
				},
				Spec: apps_v1.DeploymentSpec{
					// number of replicas is the same, but annotation is different
					Replicas: &numberOfReplicas,
					Template: core_v1.PodTemplateSpec{
						ObjectMeta: meta_v1.ObjectMeta{
							Annotations: map[string]string{
								builtin.EnvRefHashAnnotation: "disabled",
							},
						},
						Spec: core_v1.PodSpec{
							Containers: []core_v1.Container{
								{
									Name:  "c1",
									Image: "ima.ge",
								},
							},
						},
					},
				},
			},
			actual: &apps_v1.Deployment{
				TypeMeta: meta_v1.TypeMeta{
					Kind:       "Deployment",
					APIVersion: apps_v1.SchemeGroupVersion.String(),
				},
				ObjectMeta: meta_v1.ObjectMeta{
					Annotations: map[string]string{
						builtin.LastAppliedReplicasAnnotation: strconv.Itoa(int(numberOfReplicas)),
					},
				},
				Spec: apps_v1.DeploymentSpec{
					Replicas: &numberOfReplicas,
					Template: core_v1.PodTemplateSpec{
						ObjectMeta: meta_v1.ObjectMeta{
							Annotations: map[string]string{
								builtin.EnvRefHashAnnotation: "disabled",
							},
						},
						Spec: core_v1.PodSpec{
							Containers: []core_v1.Container{
								{
									Name:  "c1",
									Image: "ima.ge",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "Deployment with wrong format running replicas annotation",
			spec: &apps_v1.Deployment{
				TypeMeta: meta_v1.TypeMeta{
					Kind:       "Deployment",
					APIVersion: apps_v1.SchemeGroupVersion.String(),
				},
				ObjectMeta: meta_v1.ObjectMeta{
					Annotations: map[string]string{builtin.LastAppliedReplicasAnnotation: strconv.Itoa(int(numberOfReplicas))},
				},
				Spec: apps_v1.DeploymentSpec{
					Replicas: &numberOfReplicas,
					Template: core_v1.PodTemplateSpec{
						ObjectMeta: meta_v1.ObjectMeta{
							Annotations: map[string]string{
								builtin.EnvRefHashAnnotation: "disabled",
							},
						},
						Spec: core_v1.PodSpec{
							Containers: []core_v1.Container{
								{
									Name:  "c1",
									Image: "ima.ge",
									Ports: []core_v1.ContainerPort{
										{
											Name:          "http",
											ContainerPort: 8080,
											Protocol:      core_v1.ProtocolTCP,
										},
									},
								},
							},
							ServiceAccountName: "abc",
						},
					},
				},
			},
			actual: &apps_v1.Deployment{
				TypeMeta: meta_v1.TypeMeta{
					Kind:       "Deployment",
					APIVersion: apps_v1.SchemeGroupVersion.String(),
				},
				ObjectMeta: meta_v1.ObjectMeta{
					Annotations: map[string]string{builtin.LastAppliedReplicasAnnotation: "WrongNumberOfReplicas"},
				},
				Spec: apps_v1.DeploymentSpec{
					Replicas: &numberOfReplicas,
					Template: core_v1.PodTemplateSpec{
						ObjectMeta: meta_v1.ObjectMeta{
							Annotations: map[string]string{
								builtin.EnvRefHashAnnotation: "disabled",
							},
						},
						Spec: core_v1.PodSpec{
							Containers: []core_v1.Container{
								{
									Name:  "c1",
									Image: "ima.ge",
									Ports: []core_v1.ContainerPort{
										{
											Name:          "http",
											ContainerPort: 8080,
											Protocol:      core_v1.ProtocolTCP,
										},
									},
								},
							},
							ServiceAccountName:       "abc",
							DeprecatedServiceAccount: "abc",
						},
					},
				},
			},
		},
	}
	logger := zaptest.NewLogger(t)
	defer logger.Sync()

	for _, input := range inputs {
		input := input
		t.Run(input.name, func(t *testing.T) {
			t.Parallel()
			sc := specchecker.New(speccheckertesting.FakeStore{}, builtin.ServiceCatalogKnownTypes, builtin.MainKnownTypes)
			_, match, difference, err := sc.CompareActualVsSpec(logger, input.spec, input.actual)
			require.NoError(t, err)
			assert.False(t, match)
			assert.NotEmpty(t, difference)
		})
	}
}

func TestDoNotPanicWhenLoggingDiff(t *testing.T) {
	t.Parallel()

	logger := zaptest.NewLogger(t)
	defer logger.Sync()

	var expected, actual unstructured.Unstructured

	err := json.Unmarshal([]byte(`{ "kind": "Bundle", "apiVersion": "v1", "environment": {} }`), &expected)
	require.NoError(t, err)

	err = json.Unmarshal([]byte(`{ "kind": "Bundle", "apiVersion": "v1", "environment": "test" }`), &actual)
	require.NoError(t, err)

	sc := specchecker.New(speccheckertesting.FakeStore{}, builtin.ServiceCatalogKnownTypes, builtin.MainKnownTypes)
	_, _, _, err = sc.CompareActualVsSpec(logger, &expected, &actual)
	require.NoError(t, err)
}

func TestUpdateResourceEmptyMissingNilNoChanges(t *testing.T) {
	t.Parallel()

	inputs := map[string]func() *unstructured.Unstructured{
		"empty":   emptyMap,
		"missing": missingMap,
		"nil":     nilMap,
	}
	logger := zaptest.NewLogger(t)
	defer logger.Sync()

	for kind1, input1 := range inputs {
		for kind2, input2 := range inputs {
			actual := input1()
			spec := input2()
			t.Run(fmt.Sprintf("%s actual, %s spec", kind1, kind2), func(t *testing.T) {
				t.Parallel()
				sc := specchecker.New(speccheckertesting.FakeStore{})
				updated, match, difference, err := sc.CompareActualVsSpec(logger, spec, actual)
				require.NoError(t, err)
				assert.True(t, match)
				assert.Empty(t, difference)
				assert.True(t, equality.Semantic.DeepEqual(updated.Object, actual.Object))
			})
		}
	}
}

func emptyMap() *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":            "map1",
				"annotations":     map[string]interface{}{},
				"labels":          map[string]interface{}{},
				"ownerReferences": []interface{}{},
				"finalizers":      []interface{}{},
			},
			"data": map[string]interface{}{
				"a": "b",
			},
		},
	}
}

func missingMap() *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name": "map1",
			},
			"data": map[string]interface{}{
				"a": "b",
			},
		},
	}
}

func nilMap() *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":            "map1",
				"annotations":     nil,
				"labels":          nil,
				"ownerReferences": nil,
				"finalizers":      nil,
			},
			"data": map[string]interface{}{
				"a": "b",
			},
		},
	}
}
