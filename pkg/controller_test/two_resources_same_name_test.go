package controller_test

import (
	"context"
	"testing"

	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/smith/pkg/controller"
	smith_testing "github.com/atlassian/smith/pkg/util/testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kube_testing "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"
)

// Should detect two resources with the same name
func TestTwoResourcesWithSameName(t *testing.T) {
	t.Parallel()
	tc := testCase{
		bundle: &smith_v1.Bundle{
			ObjectMeta: meta_v1.ObjectMeta{
				Name:      bundle1,
				Namespace: testNamespace,
				UID:       bundle1uid,
			},
			Spec: smith_v1.BundleSpec{
				Resources: []smith_v1.Resource{
					{
						Name: resP1,
						Spec: smith_v1.ResourceSpec{
							Plugin: &smith_v1.PluginSpec{
								Name:       pluginConfigMapWithDeps,
								ObjectName: m1,
							},
						},
					},
					{
						Name: resP1,
						Spec: smith_v1.ResourceSpec{
							Plugin: &smith_v1.PluginSpec{
								Name:       pluginConfigMapWithDeps,
								ObjectName: m1,
							},
						},
					},
				},
			},
		},
		namespace: testNamespace,
		plugins: map[smith_v1.PluginName]func(*testing.T) testingPlugin{
			pluginConfigMapWithDeps: configMapWithDependenciesPlugin(false, false),
		},
		test: func(t *testing.T, ctx context.Context, cntrlr *controller.BundleController, tc *testCase, prepare func(ctx context.Context)) {
			prepare(ctx)
			key, err := cache.MetaNamespaceKeyFunc(tc.bundle)
			require.NoError(t, err)
			retriable, err := cntrlr.ProcessKey(tc.logger, key)
			assert.EqualError(t, err, `bundle contains two resources with the same name "`+resP1+`"`)
			assert.False(t, retriable)

			actions := tc.bundleFake.Actions()
			require.Len(t, actions, 3)
			bundleUpdate := actions[2].(kube_testing.UpdateAction)
			assert.Equal(t, testNamespace, bundleUpdate.GetNamespace())
			updateBundle := bundleUpdate.GetObject().(*smith_v1.Bundle)
			smith_testing.AssertCondition(t, updateBundle, smith_v1.BundleReady, smith_v1.ConditionFalse)
			smith_testing.AssertCondition(t, updateBundle, smith_v1.BundleInProgress, smith_v1.ConditionFalse)
			smith_testing.AssertCondition(t, updateBundle, smith_v1.BundleError, smith_v1.ConditionTrue)

			smith_testing.AssertResourceCondition(t, updateBundle, resP1, smith_v1.ResourceBlocked, smith_v1.ConditionUnknown)
			smith_testing.AssertResourceCondition(t, updateBundle, resP1, smith_v1.ResourceInProgress, smith_v1.ConditionUnknown)
			smith_testing.AssertResourceCondition(t, updateBundle, resP1, smith_v1.ResourceReady, smith_v1.ConditionUnknown)
			smith_testing.AssertResourceCondition(t, updateBundle, resP1, smith_v1.ResourceError, smith_v1.ConditionUnknown)
		},
	}
	tc.run(t)
}
