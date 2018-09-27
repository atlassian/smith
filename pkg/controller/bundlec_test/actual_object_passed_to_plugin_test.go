package bundlec_test

import (
	"testing"

	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/smith/pkg/controller/bundlec"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
)

// Should pass actual object to the plugin
func TestActualObjectPassedToPlugin(t *testing.T) {
	t.Parallel()
	tc := testCase{
		mainClientObjects: []runtime.Object{
			configMapNeedsUpdate(),
		},
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
								Name:       pluginSimpleConfigMap,
								ObjectName: mapNeedsAnUpdate,
								Spec: map[string]interface{}{
									"actualShouldExist": true,
								},
							},
						},
					},
				},
			},
		},
		appName:         testAppName,
		namespace:       testNamespace,
		expectedActions: sets.NewString("PUT=/api/v1/namespaces/" + testNamespace + "/configmaps/" + mapNeedsAnUpdate),
		testHandler: fakeActionHandler{
			response: map[path]fakeResponse{
				{
					method: "PUT",
					path:   "/api/v1/namespaces/" + testNamespace + "/configmaps/" + mapNeedsAnUpdate,
				}: configMapNeedsUpdateResponse(bundle1, bundle1uid),
			},
		},
		plugins: map[smith_v1.PluginName]func(*testing.T) testingPlugin{
			pluginSimpleConfigMap: simpleConfigMapPlugin,
		},
		pluginsShouldBeInvoked: sets.NewString(string(pluginSimpleConfigMap)),
	}
	tc.run(t)
}
