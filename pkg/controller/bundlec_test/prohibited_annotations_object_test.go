package bundlec_test

import (
	"context"
	"github.com/atlassian/smith"
	"testing"

	cond_v1 "github.com/atlassian/ctrl/apis/condition/v1"
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/smith/pkg/controller/bundlec"
	smith_testing "github.com/atlassian/smith/pkg/util/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	core_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kube_testing "k8s.io/client-go/testing"
)

// Should detect prohibited annotations in resource spec and return error
// Should work for both object and plugin spec
func TestProhibitedAnnotationsRejected(t *testing.T) {
	t.Parallel()

	cm1 := configMapNeedsUpdate()
	cm1.Annotations[smith.DeletionTimestampAnnotation] = "2006-01-02T15:04:05Z07:00"

	cm2 := configMapNeedsUpdate()
	cm2.Annotations[smith.DeletionTimestampAnnotation] = "2006-01-02T15:04:05Z07:00"

	tc := testCase{
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
						Name: "r1",
						Spec: smith_v1.ResourceSpec{
							Object: &core_v1.ConfigMap{
								TypeMeta: meta_v1.TypeMeta{
									Kind:       "ConfigMap",
									APIVersion: core_v1.SchemeGroupVersion.String(),
								},
								ObjectMeta: meta_v1.ObjectMeta{
									Name: "r1",
								},
							},
						},
					},
					{
						Name: "r2",
						Spec: smith_v1.ResourceSpec{
							Plugin: &smith_v1.PluginSpec{
								Name:       pluginMockConfigMap,
								ObjectName: "r2",
							},
						},
					},
				},
			},
		},
		appName:   testAppName,
		namespace: testNamespace,
		plugins: map[smith_v1.PluginName]func(*testing.T) testingPlugin{
			pluginMockConfigMap: mockConfigMapPlugin(cm2),
		},
		test: func(t *testing.T, ctx context.Context, cntrlr *bundlec.Controller, tc *testCase) {
			retriable, err := cntrlr.ProcessBundle(tc.logger, tc.bundle)
			assert.EqualError(t, err, `bundle contains two resources with the same name "`+resP1+`"`)
			assert.False(t, retriable)

			actions := tc.smithFake.Actions()
			require.Len(t, actions, 3)
			bundleUpdate := actions[2].(kube_testing.UpdateAction)
			assert.Equal(t, testNamespace, bundleUpdate.GetNamespace())
			updateBundle := bundleUpdate.GetObject().(*smith_v1.Bundle)
			smith_testing.AssertCondition(t, updateBundle, smith_v1.BundleReady, cond_v1.ConditionFalse)
			smith_testing.AssertCondition(t, updateBundle, smith_v1.BundleInProgress, cond_v1.ConditionFalse)
			smith_testing.AssertCondition(t, updateBundle, smith_v1.BundleError, cond_v1.ConditionTrue)

			smith_testing.AssertResourceCondition(t, updateBundle, resP1, smith_v1.ResourceBlocked, cond_v1.ConditionUnknown)
			smith_testing.AssertResourceCondition(t, updateBundle, resP1, smith_v1.ResourceInProgress, cond_v1.ConditionUnknown)
			smith_testing.AssertResourceCondition(t, updateBundle, resP1, smith_v1.ResourceReady, cond_v1.ConditionUnknown)
			smith_testing.AssertResourceCondition(t, updateBundle, resP1, smith_v1.ResourceError, cond_v1.ConditionUnknown)
		},
	}
	tc.run(t)
}
