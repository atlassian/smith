package controller_test

import (
	"context"
	"testing"

	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/smith/pkg/controller"
	smith_testing "github.com/atlassian/smith/pkg/util/testing"
	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kube_testing "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"
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
				Finalizers: []string{controller.FinalizerDeleteResources},
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
						Name:      resSb1,
						DependsOn: []smith_v1.ResourceName{resSi1},
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
									ServiceInstanceRef: sc_v1b1.LocalObjectReference{
										Name: si1,
									},
									SecretName: s1,
								},
							},
						},
					},
					{
						Name:      resSiWithDefaults,
						DependsOn: []smith_v1.ResourceName{resSb1},
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
									Parameters: &runtime.RawExtension{Raw: []byte(`{"testSchema": "{{` + resSb1 + `:bindsecret#Data.mysecret#\"nooo\"}}"}`)},
									PlanReference: sc_v1b1.PlanReference{
										ClusterServiceClassExternalName: serviceClassExternalName,
										ClusterServicePlanExternalName:  servicePlanExternalName,
									},
								},
							},
						},
					},
					{
						Name:      resPWithDefaults,
						DependsOn: []smith_v1.ResourceName{resSb1},
						Spec: smith_v1.ResourceSpec{
							Plugin: &smith_v1.PluginSpec{
								Name:       pluginConfigMapWithDeps,
								ObjectName: m1,
								Spec: map[string]interface{}{
									"p1": "{{" + resSb1 + ":bindsecret#Data.mysecret#null}}",
								},
							},
						},
					},
					{
						Name:      resSiWithoutDefaults,
						DependsOn: []smith_v1.ResourceName{resSb1},
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
									Parameters: &runtime.RawExtension{Raw: []byte(`{"testSchema": "{{` + resSb1 + `:bindsecret#Data.mysecret}}"}`)},
									PlanReference: sc_v1b1.PlanReference{
										ClusterServiceClassExternalName: serviceClassExternalName,
										ClusterServicePlanExternalName:  servicePlanExternalName,
									},
								},
							},
						},
					},
					{
						Name:      resPWithoutDefaults,
						DependsOn: []smith_v1.ResourceName{resSb1},
						Spec: smith_v1.ResourceSpec{
							Plugin: &smith_v1.PluginSpec{
								Name:       pluginConfigMapWithDeps,
								ObjectName: m1,
								Spec: map[string]interface{}{
									"p1": "{{" + resSb1 + ":bindsecret#Data.mysecret}}",
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
		test: func(t *testing.T, ctx context.Context, cntrlr *controller.BundleController, tc *testCase, prepare func(ctx context.Context)) {
			prepare(ctx)
			key, err := cache.MetaNamespaceKeyFunc(tc.bundle)
			require.NoError(t, err)
			retriable, err := cntrlr.ProcessKey(tc.logger, key)
			assert.False(t, retriable)
			assert.EqualError(t, err, `error processing resource(s): ["`+resSiWithDefaults+`" "`+resPWithDefaults+`"]`)

			actions := tc.bundleFake.Actions()
			require.Len(t, actions, 3)
			bundleUpdate := actions[2].(kube_testing.UpdateAction)
			assert.Equal(t, testNamespace, bundleUpdate.GetNamespace())
			updateBundle := bundleUpdate.GetObject().(*smith_v1.Bundle)

			resCond := smith_testing.AssertResourceCondition(t, updateBundle, resPWithDefaults, smith_v1.ResourceError, smith_v1.ConditionTrue)
			assert.Equal(t, smith_v1.ResourceReasonTerminalError, resCond.Reason)
			assert.Equal(t, "invalid spec: spec failed validation against schema: p1: Invalid type. Expected: string, given: null", resCond.Message)

			resCond = smith_testing.AssertResourceCondition(t, updateBundle, resSiWithDefaults, smith_v1.ResourceError, smith_v1.ConditionTrue)
			assert.Equal(t, smith_v1.ResourceReasonTerminalError, resCond.Reason)
			assert.Equal(t, "spec failed validation against schema: testSchema: Invalid type. Expected: boolean, given: string", resCond.Message)

			resCond = smith_testing.AssertResourceCondition(t, updateBundle, resPWithoutDefaults, smith_v1.ResourceBlocked, smith_v1.ConditionTrue)
			assert.Equal(t, smith_v1.ResourceReasonDependenciesNotReady, resCond.Reason)
			assert.Equal(t, `Not ready: ["`+resSb1+`"]`, resCond.Message)

			resCond = smith_testing.AssertResourceCondition(t, updateBundle, resPWithoutDefaults, smith_v1.ResourceBlocked, smith_v1.ConditionTrue)
			assert.Equal(t, smith_v1.ResourceReasonDependenciesNotReady, resCond.Reason)
			assert.Equal(t, `Not ready: ["`+resSb1+`"]`, resCond.Message)
		},
	}
	tc.run(t)
}
