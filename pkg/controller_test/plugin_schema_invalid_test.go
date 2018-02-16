package controller_test

import (
	"context"
	"testing"

	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/smith/pkg/controller"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
)

// Should not process plugin if specification is invalid according to the schema
func TestPluginSchemaInvalid(t *testing.T) {
	t.Parallel()
	tc := testCase{
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
						Name: resP1,
						Spec: smith_v1.ResourceSpec{
							Plugin: &smith_v1.PluginSpec{
								Name:       pluginConfigMapWithDeps,
								ObjectName: m1,
								Spec: map[string]interface{}{
									"p1": nil,
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
			// Sadly, the actual error is not current propagated
			assert.EqualError(t, err, `error processing resource(s): ["`+resP1+`"]`)
			assert.False(t, retriable)
		},
	}
	tc.run(t)
}
