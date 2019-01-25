package bundlec_test

import (
	"context"
	"testing"

	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/smith/pkg/controller/bundlec"
	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	core_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
)

func TestCleanupOfInvalidPlugin(t *testing.T) {
	t.Parallel()
	tr := true
	tc := testCase{
		apiExtClientObjects: []runtime.Object{},
		mainClientObjects: []runtime.Object{
			&core_v1.ConfigMap{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      s1,
					Namespace: testNamespace,
					UID:       s1uid,
					OwnerReferences: []meta_v1.OwnerReference{
						{
							APIVersion:         sc_v1b1.SchemeGroupVersion.String(),
							Kind:               "ServiceBinding",
							Name:               sb1,
							UID:                sb1uid,
							Controller:         &tr,
							BlockOwnerDeletion: &tr,
						},
					},
				},
			},
		},
		scClientObjects: []runtime.Object{},
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
								Name:       pluginConfigMapWithDeps,
								ObjectName: m1,
								Spec: map[string]interface{}{
									"p1": "v1", "p2": "!{bindingName}",
								},
							},
						},
					},
				},
			},
		},
		appName:              testAppName,
		namespace:            testNamespace,
		expectedActions:      sets.NewString(),
		enableServiceCatalog: true,
		testHandler: fakeActionHandler{
			response: map[path]fakeResponse{},
		},
		plugins:                map[smith_v1.PluginName]func(*testing.T) testingPlugin{},
		pluginsShouldBeInvoked: sets.NewString(),
		test: func(t *testing.T, ctx context.Context, cntrlr *bundlec.Controller, tc *testCase) {
			require.NotNil(t, tc.bundle)
			_, err := cntrlr.ProcessBundle(tc.logger, tc.bundle)
			require.Error(t, err)
			assert.EqualError(t, err, "plugin \"configMapWithDeps\" is not a valid plugin")
		},
	}
	tc.run(t)
}

func TestCleanupOfNeitherPluginOrObject(t *testing.T) {
	t.Parallel()
	tc := testCase{
		apiExtClientObjects: []runtime.Object{},
		mainClientObjects:   []runtime.Object{},
		scClientObjects:     []runtime.Object{},
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
						Name: "blah",
					},
				},
			},
		},
		appName:              testAppName,
		namespace:            testNamespace,
		expectedActions:      sets.NewString(),
		enableServiceCatalog: true,
		testHandler: fakeActionHandler{
			response: map[path]fakeResponse{},
		},
		plugins:                map[smith_v1.PluginName]func(*testing.T) testingPlugin{},
		pluginsShouldBeInvoked: sets.NewString(),
		test: func(t *testing.T, ctx context.Context, cntrlr *bundlec.Controller, tc *testCase) {
			require.NotNil(t, tc.bundle)
			_, err := cntrlr.ProcessBundle(tc.logger, tc.bundle)
			require.Error(t, err)
			assert.EqualError(t, err, "resource is neither object nor plugin")
		},
	}
	tc.run(t)
}
