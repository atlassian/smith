package speccheck

import (
	"fmt"
	"testing"

	"github.com/atlassian/smith/pkg/cleanup"
	"github.com/atlassian/smith/pkg/cleanup/types"
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
				Spec: apps_v1.DeploymentSpec{
					Template: core_v1.PodTemplateSpec{
						Spec: core_v1.PodSpec{
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
				Spec: sc_v1b1.ServiceInstanceSpec{
					PlanReference: sc_v1b1.PlanReference{
						ClusterServiceClassExternalName: "ClusterServiceClassExternalName",
						ClusterServicePlanExternalName:  "ClusterServicePlanExternalName",
						ClusterServiceClassName:         "ClusterServiceClassName",
						ClusterServicePlanName:          "ClusterServicePlanName",
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
			sc := SpecCheck{
				Logger:  logger,
				Cleaner: cleanup.New(types.ServiceCatalogKnownTypes, types.MainKnownTypes),
			}
			_, match, err := sc.CompareActualVsSpec(input.spec, input.actual)
			require.NoError(t, err)
			assert.True(t, match)
		})
	}
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
				sc := SpecCheck{
					Logger:  logger,
					Cleaner: cleanup.New(),
				}
				updated, match, err := sc.CompareActualVsSpec(spec, actual)
				require.NoError(t, err)
				assert.True(t, match)
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
