package bundlec_test

import (
	"context"
	"testing"

	cond_v1 "github.com/atlassian/ctrl/apis/condition/v1"
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

const (
	resSiWithDefaults    = "resSiWithDefaults"
	resSiWithoutDefaults = "resSiWithoutDefaults"
	resPWithDefaults     = "resPWithDefaults"
	resPWithoutDefaults  = "resPWithoutDefaults"
)

// Should not process plugin if specification is invalid according to the schema
func TestSchemaEarlyValidation(t *testing.T) {
	t.Parallel()
	tc := testCase{
		scClientObjects: []runtime.Object{
			serviceInstance(false, false, false),
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
								Spec: serviceInstanceSpec,
							},
						},
					},
					{
						Name: resSb1,
						References: []smith_v1.Reference{
							{Resource: resSi1},
						},
						Spec: smith_v1.ResourceSpec{
							Object: &sc_v1b1.ServiceBinding{
								TypeMeta: meta_v1.TypeMeta{
									Kind:       "ServiceBinding",
									APIVersion: sc_v1b1.SchemeGroupVersion.String(),
								},
								ObjectMeta: meta_v1.ObjectMeta{
									Name: sb1,
								},
								Spec: sc_v1b1.ServiceBindingSpec{
									InstanceRef: sc_v1b1.LocalObjectReference{
										Name: si1,
									},
									SecretName: s1,
								},
							},
						},
					},
					{
						Name: resSiWithDefaults,
						References: []smith_v1.Reference{
							{
								Name:     "resSb1",
								Resource: resSb1,
								Modifier: "bindsecret",
								Path:     "Data.mysecret",
								Example:  "nooo",
							},
						},
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
									Parameters: &runtime.RawExtension{Raw: []byte(`{"testSchema": "!{resSb1}"}`)},
									PlanReference: sc_v1b1.PlanReference{
										ClusterServiceClassExternalName: serviceClassExternalName,
										ClusterServicePlanExternalName:  servicePlanExternalName,
									},
								},
							},
						},
					},
					{
						Name: resPWithDefaults,
						References: []smith_v1.Reference{
							{
								Name:     "resSb1",
								Resource: resSb1,
								Modifier: "bindsecret",
								Path:     "Data.mysecret",
								Example:  true,
							},
						},
						Spec: smith_v1.ResourceSpec{
							Plugin: &smith_v1.PluginSpec{
								Name:       pluginConfigMapWithDeps,
								ObjectName: m1,
								Spec: map[string]interface{}{
									"p1": "!{resSb1}",
								},
							},
						},
					},
					{
						Name: resSiWithoutDefaults,
						References: []smith_v1.Reference{
							{
								Name:     "resSb1",
								Resource: resSb1,
								Modifier: "bindsecret",
								Path:     "Data.mysecret",
								Example:  nil,
							},
						},
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
									Parameters: &runtime.RawExtension{Raw: []byte(`{"testSchema": "!{resSb1}"}`)},
									PlanReference: sc_v1b1.PlanReference{
										ClusterServiceClassExternalName: serviceClassExternalName,
										ClusterServicePlanExternalName:  servicePlanExternalName,
									},
								},
							},
						},
					},
					{
						Name: resPWithoutDefaults,
						References: []smith_v1.Reference{
							{
								Name:     "resSb1",
								Resource: resSb1,
								Modifier: "bindsecret",
								Path:     "Data.mysecret",
								Example:  nil,
							},
						},
						Spec: smith_v1.ResourceSpec{
							Plugin: &smith_v1.PluginSpec{
								Name:       pluginConfigMapWithDeps,
								ObjectName: m1,
								Spec: map[string]interface{}{
									"p1": "!{resSb1}",
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
			assert.EqualError(t, err, `error processing resource(s): ["`+resSiWithDefaults+`" "`+resPWithDefaults+`"]`)

			actions := tc.smithFake.Actions()
			require.Len(t, actions, 3)
			bundleUpdate := actions[2].(kube_testing.UpdateAction)
			assert.Equal(t, testNamespace, bundleUpdate.GetNamespace())
			updateBundle := bundleUpdate.GetObject().(*smith_v1.Bundle)

			resCond := smith_testing.AssertResourceCondition(t, updateBundle, resPWithDefaults, smith_v1.ResourceError, cond_v1.ConditionTrue)
			if resCond != nil {
				assert.Equal(t, smith_v1.ResourceReasonTerminalError, resCond.Reason)
				assert.Equal(t, "spec failed validation against schema: p1: Invalid type. Expected: string, given: boolean", resCond.Message)
			}
			resCond = smith_testing.AssertResourceCondition(t, updateBundle, resSiWithDefaults, smith_v1.ResourceError, cond_v1.ConditionTrue)
			if resCond != nil {
				assert.Equal(t, smith_v1.ResourceReasonTerminalError, resCond.Reason)
				assert.Equal(t, "spec failed validation against schema: testSchema: Invalid type. Expected: boolean, given: string", resCond.Message)
			}
			resCond = smith_testing.AssertResourceCondition(t, updateBundle, resPWithoutDefaults, smith_v1.ResourceBlocked, cond_v1.ConditionTrue)
			if resCond != nil {
				assert.Equal(t, smith_v1.ResourceReasonDependenciesNotReady, resCond.Reason)
				assert.Equal(t, `Not ready: ["`+resSb1+`"]`, resCond.Message)
			}
			resCond = smith_testing.AssertResourceCondition(t, updateBundle, resPWithoutDefaults, smith_v1.ResourceBlocked, cond_v1.ConditionTrue)
			if resCond != nil {
				assert.Equal(t, smith_v1.ResourceReasonDependenciesNotReady, resCond.Reason)
				assert.Equal(t, `Not ready: ["`+resSb1+`"]`, resCond.Message)
			}
		},
	}
	tc.run(t)
}
