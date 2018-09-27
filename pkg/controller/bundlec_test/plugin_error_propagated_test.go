package bundlec_test

import (
	"context"
	"testing"

	cond_v1 "github.com/atlassian/ctrl/apis/condition/v1"
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/smith/pkg/controller/bundlec"
	smith_testing "github.com/atlassian/smith/pkg/util/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	kube_testing "k8s.io/client-go/testing"
)

// Should propagate plugin error into resource's status
func TestPluginErrorPropagated(t *testing.T) {
	t.Parallel()
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
						Name: resP1,
						Spec: smith_v1.ResourceSpec{
							Plugin: &smith_v1.PluginSpec{
								Name:       pluginFailing,
								ObjectName: m1,
							},
						},
					},
				},
			},
		},
		plugins: map[smith_v1.PluginName]func(*testing.T) testingPlugin{
			pluginFailing: newFailingPlugin,
		},
		pluginsShouldBeInvoked: sets.NewString(string(pluginFailing)),
		test: func(t *testing.T, ctx context.Context, cntrlr *bundlec.Controller, tc *testCase) {
			retriable, err := cntrlr.ProcessBundle(tc.logger, tc.bundle)
			assert.EqualError(t, err, `error processing resource(s): ["`+resP1+`"]`)
			assert.False(t, retriable)

			actions := tc.smithFake.Actions()
			require.Len(t, actions, 3)
			bundleUpdate := actions[2].(kube_testing.UpdateAction)
			assert.Equal(t, testNamespace, bundleUpdate.GetNamespace())
			updateBundle := bundleUpdate.GetObject().(*smith_v1.Bundle)

			resCond := smith_testing.AssertResourceCondition(t, updateBundle, resP1, smith_v1.ResourceError, cond_v1.ConditionTrue)
			if resCond != nil {
				assert.Equal(t, smith_v1.ResourceReasonTerminalError, resCond.Reason)
				assert.Equal(t, "plugin failed as it should. BOOM!", resCond.Message)
			}
		},
	}
	tc.run(t)
}
