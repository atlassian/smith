package bundlec_test

import (
	"context"
	"testing"

	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/smith/pkg/controller/bundlec"
	smith_testing "github.com/atlassian/smith/pkg/util/testing"
	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kube_testing "k8s.io/client-go/testing"
)

// Should not process plugin if specification is invalid according to the schema
func TestServiceInstanceSchemaInvalid(t *testing.T) {
	t.Parallel()
	tc := testCase{
		scClientObjects: []runtime.Object{
			serviceInstance(false, false, true),
		},
		enableServiceCatalog: true,
		bundle: &smith_v1.Bundle{
			ObjectMeta: meta_v1.ObjectMeta{
				Name:       bundle1,
				Namespace:  testNamespace,
				UID:        bundle1uid,
				Finalizers: []string{bundlec.FinalizerDeleteResources},
			},
			Spec: smith_v1.BundleSpec{
				Resources: []smith_v1.Resource{
					{
						Name: resSi1,
						Spec: smith_v1.ResourceSpec{
							Object: &sc_v1b1.ServiceInstance{
								TypeMeta: meta_v1.TypeMeta{
									Kind:       "ServiceInstance",
									APIVersion: sc_v1b1.SchemeGroupVersion.String(),
								},
								ObjectMeta: meta_v1.ObjectMeta{
									Name: si1,
								},
								Spec: sc_v1b1.ServiceInstanceSpec{
									Parameters: &runtime.RawExtension{Raw: []byte(`{"testSchema": "invalid"}`)},
									PlanReference: sc_v1b1.PlanReference{
										ClusterServiceClassExternalName: serviceClassExternalName,
										ClusterServicePlanExternalName:  servicePlanExternalName,
									},
								},
							},
						},
					},
					{
						Name: resSi2,
						Spec: smith_v1.ResourceSpec{
							Object: &sc_v1b1.ServiceInstance{
								TypeMeta: meta_v1.TypeMeta{
									Kind:       "ServiceInstance",
									APIVersion: sc_v1b1.SchemeGroupVersion.String(),
								},
								ObjectMeta: meta_v1.ObjectMeta{
									Name: si2,
								},
								Spec: sc_v1b1.ServiceInstanceSpec{
									Parameters: &runtime.RawExtension{Raw: []byte(`{"testSchema": "invalid"}`)},
									PlanReference: sc_v1b1.PlanReference{
										ClusterServiceClassExternalID: serviceClassNameAndID,
										ClusterServicePlanExternalID:  servicePlanNameAndID,
									},
								},
							},
						},
					},
				},
			},
		},
		plugins: map[smith_v1.PluginName]func(*testing.T) testingPlugin{
			pluginConfigMapWithDeps: configMapWithDependenciesPlugin(false, false),
		},
		test: func(t *testing.T, ctx context.Context, cntrlr *bundlec.Controller, tc *testCase) {
			retriable, err := cntrlr.ProcessBundle(tc.logger, tc.bundle)
			assert.False(t, retriable)
			assert.EqualError(t, err, `error processing resource(s): ["`+resSi1+`" "`+resSi2+`"]`)

			actions := tc.smithFake.Actions()
			require.Len(t, actions, 3)
			bundleUpdate := actions[2].(kube_testing.UpdateAction)
			assert.Equal(t, testNamespace, bundleUpdate.GetNamespace())
			updateBundle := bundleUpdate.GetObject().(*smith_v1.Bundle)

			resCond := smith_testing.AssertResourceCondition(t, updateBundle, resSi1, smith_v1.ResourceError, smith_v1.ConditionTrue)
			if resCond != nil {
				assert.Equal(t, smith_v1.ResourceReasonTerminalError, resCond.Reason)
				assert.Equal(t, "spec failed validation against schema: testSchema: Invalid type. Expected: boolean, given: string", resCond.Message)
			}
			resCond = smith_testing.AssertResourceCondition(t, updateBundle, resSi2, smith_v1.ResourceError, smith_v1.ConditionTrue)
			if resCond != nil {
				assert.Equal(t, smith_v1.ResourceReasonTerminalError, resCond.Reason)
				assert.Equal(t, "spec failed validation against schema: testSchema: Invalid type. Expected: boolean, given: string", resCond.Message)
			}
		},
	}
	tc.run(t)
}
