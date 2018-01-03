package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/atlassian/smith"
	"github.com/atlassian/smith/examples/sleeper"
	"github.com/atlassian/smith/examples/sleeper/pkg/apis/sleeper/v1"
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/smith/pkg/cleanup"
	clean_types "github.com/atlassian/smith/pkg/cleanup/types"
	"github.com/atlassian/smith/pkg/client"
	smithFake "github.com/atlassian/smith/pkg/client/clientset_generated/clientset/fake"
	"github.com/atlassian/smith/pkg/client/smart"
	"github.com/atlassian/smith/pkg/plugin"
	"github.com/atlassian/smith/pkg/readychecker"
	ready_types "github.com/atlassian/smith/pkg/readychecker/types"
	"github.com/atlassian/smith/pkg/resources/apitypes"
	"github.com/atlassian/smith/pkg/speccheck"
	"github.com/atlassian/smith/pkg/store"
	"github.com/atlassian/smith/pkg/util"
	smith_testing "github.com/atlassian/smith/pkg/util/testing"

	"github.com/ash2k/stager"
	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	scClientset "github.com/kubernetes-incubator/service-catalog/pkg/client/clientset_generated/clientset"
	scFake "github.com/kubernetes-incubator/service-catalog/pkg/client/clientset_generated/clientset/fake"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	core_v1 "k8s.io/api/core/v1"
	apiext_v1b1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	crdFake "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	apiext_v1b1inf "k8s.io/apiextensions-apiserver/pkg/client/informers/externalversions/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/meta"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/dynamic"
	mainFake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	kube_testing "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

type reaction struct {
	verb     string
	resource string
	reactor  func(*testing.T) kube_testing.ReactionFunc
}

type testCase struct {
	mainClientObjects  []runtime.Object
	mainReactors       []reaction
	smithClientObjects []runtime.Object
	smithReactors      []reaction
	crdClientObjects   []runtime.Object
	crdReactors        []reaction
	scClientObjects    []runtime.Object
	scReactors         []reaction
	bundle             *smith_v1.Bundle
	namespace          string

	expectedActions        sets.String
	enableServiceCatalog   bool
	testHandler            fakeActionHandler
	test                   func(*testing.T, context.Context, *BundleController, *testCase)
	plugins                map[smith_v1.PluginName]func(*testing.T) testingPlugin
	pluginsShouldBeInvoked sets.String

	mainFake           *kube_testing.Fake
	bundleFake         *kube_testing.Fake
	crdFake            *kube_testing.Fake
	scFake             *kube_testing.Fake
	pluginsConstructed []testingPlugin
}

const (
	testNamespace = "test-namespace"

	resSb1 = "resSb1"
	sb1    = "sb1"

	pluginSimpleConfigMap   = "simpleConfigMap"
	pluginConfigMapWithDeps = "configMapWithDeps"
)

func TestController(t *testing.T) {
	t.Parallel()
	tr := true
	testcases := map[string]*testCase{
		"deletes owned object that is not in bundle": &testCase{
			mainClientObjects: []runtime.Object{
				&core_v1.ConfigMap{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "map1",
						Namespace: testNamespace,
						OwnerReferences: []meta_v1.OwnerReference{
							{
								APIVersion:         smith_v1.BundleResourceGroupVersion,
								Kind:               smith_v1.BundleResourceKind,
								Name:               "bundle1",
								UID:                "uid123",
								Controller:         &tr,
								BlockOwnerDeletion: &tr,
							},
						},
					},
				},
			},
			bundle: &smith_v1.Bundle{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "bundle1",
					Namespace: testNamespace,
					UID:       "uid123",
				},
			},
			expectedActions: sets.NewString("DELETE=/api/v1/namespaces/" + testNamespace + "/configmaps/map1"),
			namespace:       meta_v1.NamespaceAll,
		},
		"can list crds in another namespace": &testCase{
			crdClientObjects: []runtime.Object{
				SleeperCrdWithStatus(),
			},
			expectedActions: sets.NewString(
				"GET=/apis/"+v1.SleeperResourceGroupVersion+"/namespaces/"+testNamespace+"/"+v1.SleeperResourcePlural+
					"=resourceVersion=0",
				"GET=/apis/"+v1.SleeperResourceGroupVersion+"/namespaces/"+testNamespace+"/"+v1.SleeperResourcePlural+
					"=watch",
			),
			namespace: testNamespace,
			test: func(t *testing.T, ctx context.Context, bundleController *BundleController, testcase *testCase) {
				// TODO Test is broken (timeouts, and passes without Run ... ?)
				//subContext, _ := context.WithTimeout(ctx, 1000*time.Millisecond)
				//bundleController.Run(subContext)
			},
			testHandler: fakeActionHandler{
				response: map[path]fakeResponse{
					{
						method: "GET",
						watch:  true,
						path:   "/apis/" + v1.SleeperResourceGroupVersion + "/namespaces/" + testNamespace + "/" + v1.SleeperResourcePlural,
					}: {
						statusCode: http.StatusOK,
						content:    []byte(`{"type": "ADDED", "object": { "kind": "Unknown" } }`),
					},
				},
			},
		},
		"actual object is passed to the plugin": &testCase{
			mainClientObjects: []runtime.Object{
				&core_v1.ConfigMap{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "map1",
						Namespace: testNamespace,
						OwnerReferences: []meta_v1.OwnerReference{
							{
								APIVersion:         smith_v1.BundleResourceGroupVersion,
								Kind:               smith_v1.BundleResourceKind,
								Name:               "bundle1",
								UID:                "uid123",
								Controller:         &tr,
								BlockOwnerDeletion: &tr,
							},
						},
					},
				},
			},
			bundle: &smith_v1.Bundle{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "bundle1",
					Namespace: testNamespace,
					UID:       "uid123",
				},
				Spec: smith_v1.BundleSpec{
					Resources: []smith_v1.Resource{
						{
							Name: "p1",
							Spec: smith_v1.ResourceSpec{
								Plugin: &smith_v1.PluginSpec{
									Name:       pluginSimpleConfigMap,
									ObjectName: "map1",
									Spec: map[string]interface{}{
										"actualShouldExist": true,
									},
								},
							},
						},
					},
				},
			},
			namespace:       testNamespace,
			expectedActions: sets.NewString("PUT=/api/v1/namespaces/" + testNamespace + "/configmaps/map1"),
			testHandler: fakeActionHandler{
				response: map[path]fakeResponse{
					{
						method: "PUT",
						path:   "/api/v1/namespaces/" + testNamespace + "/configmaps/map1",
					}: {
						statusCode: http.StatusOK,
						content: []byte(`{
							"apiVersion": "v1",
							"kind": "ConfigMap",
							"metadata": {
								"name": "map1",
								"namespace": "` + testNamespace + `",
								"uid": "map-update-uid",
								"labels": {
									"` + smith.BundleNameLabel + `": "bundle1"
								},
								"ownerReferences": [{
									"apiVersion": "` + smith_v1.BundleResourceGroupVersion + `",
									"kind": "` + smith_v1.BundleResourceKind + `",
									"name": "bundle1",
									"uid": "uid123",
									"controller": true,
									"blockOwnerDeletion": true
								}] }
							}`),
					},
				},
			},
			plugins: map[smith_v1.PluginName]func(*testing.T) testingPlugin{
				pluginSimpleConfigMap: simpleConfigMapPlugin,
			},
			pluginsShouldBeInvoked: sets.NewString(pluginSimpleConfigMap),
		},
		"plugin spec is processed": &testCase{
			mainClientObjects: []runtime.Object{
				&core_v1.Secret{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "s1",
						Namespace: testNamespace,
						OwnerReferences: []meta_v1.OwnerReference{
							{
								APIVersion:         sc_v1b1.SchemeGroupVersion.String(),
								Kind:               "ServiceBinding",
								Name:               sb1,
								UID:                types.UID("sb1-uid"),
								Controller:         &tr,
								BlockOwnerDeletion: &tr,
							},
						},
					},
					Data: map[string][]byte{
						"data": []byte("bla"),
					},
					Type: core_v1.SecretTypeOpaque,
				},
			},
			scClientObjects: []runtime.Object{
				&sc_v1b1.ServiceInstance{
					TypeMeta: meta_v1.TypeMeta{
						Kind:       "ServiceInstance",
						APIVersion: sc_v1b1.SchemeGroupVersion.String(),
					},
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "si1",
						Namespace: testNamespace,
						UID:       types.UID("si1-uid"),
						Labels: map[string]string{
							smith.BundleNameLabel: "bundle1",
						},
						OwnerReferences: []meta_v1.OwnerReference{
							{
								APIVersion:         smith_v1.BundleResourceGroupVersion,
								Kind:               smith_v1.BundleResourceKind,
								Name:               "bundle1",
								UID:                "uid123",
								Controller:         &tr,
								BlockOwnerDeletion: &tr,
							},
						},
					},
					Status: sc_v1b1.ServiceInstanceStatus{
						Conditions: []sc_v1b1.ServiceInstanceCondition{
							{
								Type:   sc_v1b1.ServiceInstanceConditionReady,
								Status: sc_v1b1.ConditionTrue,
							},
						},
					},
				},
				&sc_v1b1.ServiceBinding{
					TypeMeta: meta_v1.TypeMeta{
						Kind:       "ServiceBinding",
						APIVersion: sc_v1b1.SchemeGroupVersion.String(),
					},
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      sb1,
						Namespace: testNamespace,
						UID:       types.UID("sb1-uid"),
						Labels: map[string]string{
							smith.BundleNameLabel: "bundle1",
						},
						OwnerReferences: []meta_v1.OwnerReference{
							{
								APIVersion:         smith_v1.BundleResourceGroupVersion,
								Kind:               smith_v1.BundleResourceKind,
								Name:               "bundle1",
								UID:                "uid123",
								Controller:         &tr,
								BlockOwnerDeletion: &tr,
							},
							{
								APIVersion:         sc_v1b1.SchemeGroupVersion.String(),
								Kind:               "ServiceInstance",
								Name:               "si1",
								UID:                "si1-uid",
								BlockOwnerDeletion: &tr,
							},
						},
					},
					Spec: sc_v1b1.ServiceBindingSpec{
						ServiceInstanceRef: sc_v1b1.LocalObjectReference{
							Name: "si1",
						},
						SecretName: "s1",
					},
					Status: sc_v1b1.ServiceBindingStatus{
						Conditions: []sc_v1b1.ServiceBindingCondition{
							{
								Type:   sc_v1b1.ServiceBindingConditionReady,
								Status: sc_v1b1.ConditionTrue,
							},
						},
					},
				},
			},
			bundle: &smith_v1.Bundle{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "bundle1",
					Namespace: testNamespace,
					UID:       "uid123",
				},
				Spec: smith_v1.BundleSpec{
					Resources: []smith_v1.Resource{
						{
							Name: "si1",
							Spec: smith_v1.ResourceSpec{
								Object: &sc_v1b1.ServiceInstance{
									TypeMeta: meta_v1.TypeMeta{
										Kind:       "ServiceInstance",
										APIVersion: sc_v1b1.SchemeGroupVersion.String(),
									},
									ObjectMeta: meta_v1.ObjectMeta{
										Name: "si1",
									},
								},
							},
						},
						{
							Name: resSb1,
							DependsOn: []smith_v1.ResourceName{
								"si1",
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
										ServiceInstanceRef: sc_v1b1.LocalObjectReference{
											Name: "si1",
										},
										SecretName: "s1",
									},
								},
							},
						},
						{
							Name: "p1",
							DependsOn: []smith_v1.ResourceName{
								resSb1,
							},
							Spec: smith_v1.ResourceSpec{
								Plugin: &smith_v1.PluginSpec{
									Name:       pluginConfigMapWithDeps,
									ObjectName: "m1",
									Spec: map[string]interface{}{
										"p1": "v1", "p2": "{{" + resSb1 + "#metadata.name}}",
									},
								},
							},
						},
					},
				},
			},
			namespace:            testNamespace,
			expectedActions:      sets.NewString("POST=/api/v1/namespaces/" + testNamespace + "/configmaps"),
			enableServiceCatalog: true,
			testHandler: fakeActionHandler{
				response: map[path]fakeResponse{
					{
						method: "POST",
						path:   "/api/v1/namespaces/" + testNamespace + "/configmaps",
					}: {
						statusCode: http.StatusCreated,
						content: []byte(`{
							"apiVersion": "v1",
							"kind": "ConfigMap",
							"metadata": {
								"name": "m1",
								"namespace": "` + testNamespace + `",
								"uid": "map-update-uid",
								"labels": {
									"` + smith.BundleNameLabel + `": "bundle1"
								},
								"ownerReferences": [
									{
										"apiVersion": "` + smith_v1.BundleResourceGroupVersion + `",
										"kind": "` + smith_v1.BundleResourceKind + `",
										"name": "bundle1",
										"uid": "uid123",
										"controller": true,
										"blockOwnerDeletion": true
									},
									{
										"apiVersion": "` + sc_v1b1.SchemeGroupVersion.String() + `",
										"kind": "ServiceBinding",
										"name": "sb1",
										"uid": "sb1-uid",
										"blockOwnerDeletion": true
									}
								] }
							}`),
					},
				},
			},
			plugins: map[smith_v1.PluginName]func(*testing.T) testingPlugin{
				pluginConfigMapWithDeps: configMapWithDependenciesPlugin,
			},
			pluginsShouldBeInvoked: sets.NewString(pluginConfigMapWithDeps),
		},
		"plugin not processed if spec invalid according to schema": &testCase{
			bundle: &smith_v1.Bundle{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "bundle1",
					Namespace: testNamespace,
					UID:       "uid123",
				},
				Spec: smith_v1.BundleSpec{
					Resources: []smith_v1.Resource{
						{
							Name: "p1",
							Spec: smith_v1.ResourceSpec{
								Plugin: &smith_v1.PluginSpec{
									Name:       pluginConfigMapWithDeps,
									ObjectName: "m1",
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
				pluginConfigMapWithDeps: configMapWithDependenciesPlugin,
			},
			test: func(t *testing.T, ctx context.Context, cntrlr *BundleController, tc *testCase) {
				key, err := cache.MetaNamespaceKeyFunc(tc.bundle)
				require.NoError(t, err)
				retriable, err := cntrlr.processKey(key)
				// Sadly, the actual error is not current propagated
				require.EqualError(t, err, `error processing resource(s): ["p1"]`)
				assert.False(t, retriable)
			},
		},
		"no resource creates/updates/deletes done after error is encountered": &testCase{
			mainClientObjects: []runtime.Object{
				&core_v1.ConfigMap{
					TypeMeta: meta_v1.TypeMeta{
						Kind:       "ConfigMap",
						APIVersion: core_v1.SchemeGroupVersion.String(),
					},
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "map-not-in-the-bundle-anymore-needs-delete",
						Namespace: testNamespace,
						UID:       types.UID("map-delete-uid"),
						Labels: map[string]string{
							smith.BundleNameLabel: "bundle1",
						},
						OwnerReferences: []meta_v1.OwnerReference{
							{
								APIVersion:         smith_v1.BundleResourceGroupVersion,
								Kind:               smith_v1.BundleResourceKind,
								Name:               "bundle1",
								UID:                "uid123",
								Controller:         &tr,
								BlockOwnerDeletion: &tr,
							},
						},
					},
				},
				&core_v1.ConfigMap{
					TypeMeta: meta_v1.TypeMeta{
						Kind:       "ConfigMap",
						APIVersion: core_v1.SchemeGroupVersion.String(),
					},
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "map-needs-update",
						Namespace: testNamespace,
						UID:       types.UID("map-update-uid"),
						// Labels missing - needs an update
						OwnerReferences: []meta_v1.OwnerReference{
							{
								APIVersion:         smith_v1.BundleResourceGroupVersion,
								Kind:               smith_v1.BundleResourceKind,
								Name:               "bundle1",
								UID:                "uid123",
								Controller:         &tr,
								BlockOwnerDeletion: &tr,
							},
						},
					},
					Data: map[string]string{
						"delete": "this key",
					},
				},
			},
			scClientObjects: []runtime.Object{
				&sc_v1b1.ServiceInstance{
					TypeMeta: meta_v1.TypeMeta{
						Kind:       "ServiceInstance",
						APIVersion: sc_v1b1.SchemeGroupVersion.String(),
					},
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "si1",
						Namespace: testNamespace,
						UID:       types.UID("si1-uid"),
						Labels: map[string]string{
							smith.BundleNameLabel: "bundle1",
						},
						OwnerReferences: []meta_v1.OwnerReference{
							{
								APIVersion:         smith_v1.BundleResourceGroupVersion,
								Kind:               smith_v1.BundleResourceKind,
								Name:               "bundle1",
								UID:                "uid123",
								Controller:         &tr,
								BlockOwnerDeletion: &tr,
							},
						},
					},
					Status: sc_v1b1.ServiceInstanceStatus{
						Conditions: []sc_v1b1.ServiceInstanceCondition{
							{
								Type:    sc_v1b1.ServiceInstanceConditionFailed,
								Status:  sc_v1b1.ConditionTrue,
								Reason:  "BlaBla",
								Message: "Oh no!",
							},
						},
					},
				},
			},
			bundle: &smith_v1.Bundle{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "bundle1",
					Namespace: testNamespace,
					UID:       "uid123",
				},
				Spec: smith_v1.BundleSpec{
					Resources: []smith_v1.Resource{
						{
							Name: "si1",
							Spec: smith_v1.ResourceSpec{
								Object: &sc_v1b1.ServiceInstance{
									TypeMeta: meta_v1.TypeMeta{
										Kind:       "ServiceInstance",
										APIVersion: sc_v1b1.SchemeGroupVersion.String(),
									},
									ObjectMeta: meta_v1.ObjectMeta{
										Name: "si1",
									},
								},
							},
						},
						{
							Name: "map1",
							Spec: smith_v1.ResourceSpec{
								Object: &core_v1.ConfigMap{
									TypeMeta: meta_v1.TypeMeta{
										Kind:       "ConfigMap",
										APIVersion: core_v1.SchemeGroupVersion.String(),
									},
									ObjectMeta: meta_v1.ObjectMeta{
										Name: "map1",
									},
								},
							},
						},
					},
				},
			},
			namespace:            testNamespace,
			enableServiceCatalog: true,
			test: func(t *testing.T, ctx context.Context, cntrlr *BundleController, tc *testCase) {
				key, err := cache.MetaNamespaceKeyFunc(tc.bundle)
				require.NoError(t, err)
				retriable, err := cntrlr.processKey(key)
				require.EqualError(t, err, `error processing resource(s): ["si1"]`)
				assert.False(t, retriable)
				actions := tc.bundleFake.Actions()
				require.Len(t, actions, 3)
				bundleUpdate := actions[2].(kube_testing.UpdateAction)
				assert.Equal(t, testNamespace, bundleUpdate.GetNamespace())
				updateBundle := bundleUpdate.GetObject().(*smith_v1.Bundle)
				smith_testing.AssertCondition(t, updateBundle, smith_v1.BundleReady, smith_v1.ConditionFalse)
				smith_testing.AssertCondition(t, updateBundle, smith_v1.BundleInProgress, smith_v1.ConditionFalse)
				smith_testing.AssertCondition(t, updateBundle, smith_v1.BundleError, smith_v1.ConditionTrue)

				smith_testing.AssertResourceCondition(t, updateBundle, "si1", smith_v1.ResourceBlocked, smith_v1.ConditionFalse)
				smith_testing.AssertResourceCondition(t, updateBundle, "si1", smith_v1.ResourceInProgress, smith_v1.ConditionFalse)
				smith_testing.AssertResourceCondition(t, updateBundle, "si1", smith_v1.ResourceReady, smith_v1.ConditionFalse)
				resCond := smith_testing.AssertResourceCondition(t, updateBundle, "si1", smith_v1.ResourceError, smith_v1.ConditionTrue)
				assert.Equal(t, smith_v1.ResourceReasonTerminalError, resCond.Reason)
				assert.Equal(t, "readiness check failed: BlaBla: Oh no!", resCond.Message)

				resCond = smith_testing.AssertResourceCondition(t, updateBundle, "map1", smith_v1.ResourceBlocked, smith_v1.ConditionTrue)
				assert.Equal(t, smith_v1.ResourceReasonOtherResourceError, resCond.Reason)
				assert.Equal(t, "Some other resource is in error state", resCond.Message)
				smith_testing.AssertResourceCondition(t, updateBundle, "map1", smith_v1.ResourceInProgress, smith_v1.ConditionFalse)
				smith_testing.AssertResourceCondition(t, updateBundle, "map1", smith_v1.ResourceReady, smith_v1.ConditionFalse)
				smith_testing.AssertResourceCondition(t, updateBundle, "map1", smith_v1.ResourceError, smith_v1.ConditionFalse)
			},
		},
		"resources are not deleted if bundle is in progress": &testCase{
			mainClientObjects: []runtime.Object{
				&core_v1.ConfigMap{
					TypeMeta: meta_v1.TypeMeta{
						Kind:       "ConfigMap",
						APIVersion: core_v1.SchemeGroupVersion.String(),
					},
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "map-not-in-the-bundle-anymore-needs-delete",
						Namespace: testNamespace,
						UID:       types.UID("map-delete-uid"),
						Labels: map[string]string{
							smith.BundleNameLabel: "bundle1",
						},
						OwnerReferences: []meta_v1.OwnerReference{
							{
								APIVersion:         smith_v1.BundleResourceGroupVersion,
								Kind:               smith_v1.BundleResourceKind,
								Name:               "bundle1",
								UID:                "uid123",
								Controller:         &tr,
								BlockOwnerDeletion: &tr,
							},
						},
					},
				},
			},
			scClientObjects: []runtime.Object{
				&sc_v1b1.ServiceInstance{
					TypeMeta: meta_v1.TypeMeta{
						Kind:       "ServiceInstance",
						APIVersion: sc_v1b1.SchemeGroupVersion.String(),
					},
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "si1",
						Namespace: testNamespace,
						UID:       types.UID("si1-uid"),
						Labels: map[string]string{
							smith.BundleNameLabel: "bundle1",
						},
						OwnerReferences: []meta_v1.OwnerReference{
							{
								APIVersion:         smith_v1.BundleResourceGroupVersion,
								Kind:               smith_v1.BundleResourceKind,
								Name:               "bundle1",
								UID:                "uid123",
								Controller:         &tr,
								BlockOwnerDeletion: &tr,
							},
						},
					},
					Status: sc_v1b1.ServiceInstanceStatus{
						Conditions: []sc_v1b1.ServiceInstanceCondition{
							{
								Type:    sc_v1b1.ServiceInstanceConditionReady,
								Status:  sc_v1b1.ConditionFalse,
								Reason:  "WorkingOnIt",
								Message: "Doing something",
							},
						},
					},
				},
			},
			bundle: &smith_v1.Bundle{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "bundle1",
					Namespace: testNamespace,
					UID:       "uid123",
				},
				Spec: smith_v1.BundleSpec{
					Resources: []smith_v1.Resource{
						{
							Name: "si1",
							Spec: smith_v1.ResourceSpec{
								Object: &sc_v1b1.ServiceInstance{
									TypeMeta: meta_v1.TypeMeta{
										Kind:       "ServiceInstance",
										APIVersion: sc_v1b1.SchemeGroupVersion.String(),
									},
									ObjectMeta: meta_v1.ObjectMeta{
										Name: "si1",
									},
								},
							},
						},
					},
				},
			},
			namespace:            testNamespace,
			enableServiceCatalog: true,
		},
		"detect infinite update cycles": &testCase{
			mainClientObjects: []runtime.Object{
				&core_v1.ConfigMap{
					TypeMeta: meta_v1.TypeMeta{
						Kind:       "ConfigMap",
						APIVersion: core_v1.SchemeGroupVersion.String(),
					},
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "map-needs-update",
						Namespace: testNamespace,
						UID:       types.UID("map-update-uid"),
						// Labels missing - needs an update
						OwnerReferences: []meta_v1.OwnerReference{
							{
								APIVersion:         smith_v1.BundleResourceGroupVersion,
								Kind:               smith_v1.BundleResourceKind,
								Name:               "bundle1",
								UID:                "uid123",
								Controller:         &tr,
								BlockOwnerDeletion: &tr,
							},
						},
					},
				},
			},
			bundle: &smith_v1.Bundle{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "bundle1",
					Namespace: testNamespace,
					UID:       "uid123",
				},
				Spec: smith_v1.BundleSpec{
					Resources: []smith_v1.Resource{
						{
							Name: "map-needs-update",
							Spec: smith_v1.ResourceSpec{
								Object: &core_v1.ConfigMap{
									TypeMeta: meta_v1.TypeMeta{
										Kind:       "ConfigMap",
										APIVersion: core_v1.SchemeGroupVersion.String(),
									},
									ObjectMeta: meta_v1.ObjectMeta{
										Name: "map-needs-update",
									},
								},
							},
						},
					},
				},
			},
			namespace:       testNamespace,
			expectedActions: sets.NewString("PUT=/api/v1/namespaces/" + testNamespace + "/configmaps/map-needs-update"),
			testHandler: fakeActionHandler{
				response: map[path]fakeResponse{
					{
						method: "PUT",
						path:   "/api/v1/namespaces/" + testNamespace + "/configmaps/map-needs-update",
					}: {
						statusCode: http.StatusOK,
						content: []byte(`{
							"apiVersion": "v1",
							"kind": "ConfigMap",
							"metadata": {
								"name": "map1",
								"namespace": "` + testNamespace + `",
								"uid": "map-update-uid",
								"ownerReferences": [{
									"apiVersion": "` + smith_v1.BundleResourceGroupVersion + `",
									"kind": "` + smith_v1.BundleResourceKind + `",
									"name": "bundle1",
									"uid": "uid123",
									"controller": true,
									"blockOwnerDeletion": true
								}] }
							}`),
					},
				},
			},
			test: func(t *testing.T, ctx context.Context, cntrlr *BundleController, tc *testCase) {
				key, err := cache.MetaNamespaceKeyFunc(tc.bundle)
				require.NoError(t, err)
				retriable, err := cntrlr.processKey(key)
				require.EqualError(t, err, `error processing resource(s): ["map-needs-update"]`)
				assert.False(t, retriable)
				actions := tc.bundleFake.Actions()
				require.Len(t, actions, 3)
				bundleUpdate := actions[2].(kube_testing.UpdateAction)
				assert.Equal(t, testNamespace, bundleUpdate.GetNamespace())
				updateBundle := bundleUpdate.GetObject().(*smith_v1.Bundle)
				smith_testing.AssertCondition(t, updateBundle, smith_v1.BundleReady, smith_v1.ConditionFalse)
				smith_testing.AssertCondition(t, updateBundle, smith_v1.BundleInProgress, smith_v1.ConditionFalse)
				smith_testing.AssertCondition(t, updateBundle, smith_v1.BundleError, smith_v1.ConditionTrue)

				smith_testing.AssertResourceCondition(t, updateBundle, "map-needs-update", smith_v1.ResourceBlocked, smith_v1.ConditionFalse)
				smith_testing.AssertResourceCondition(t, updateBundle, "map-needs-update", smith_v1.ResourceInProgress, smith_v1.ConditionFalse)
				smith_testing.AssertResourceCondition(t, updateBundle, "map-needs-update", smith_v1.ResourceReady, smith_v1.ConditionFalse)
				resCond := smith_testing.AssertResourceCondition(t, updateBundle, "map-needs-update", smith_v1.ResourceError, smith_v1.ConditionTrue)
				assert.Equal(t, smith_v1.ResourceReasonTerminalError, resCond.Reason)
				assert.Equal(t, "specification of the created/updated object does not match the desired spec", resCond.Message)
			},
		},
	}
	for name, tc := range testcases {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			defer tc.verifyActions(t)
			defer tc.verifyPlugins(t)
			tc.run(t)
		})
	}
}

func (tc *testCase) run(t *testing.T) {
	mainClient := mainFake.NewSimpleClientset(tc.mainClientObjects...)
	tc.mainFake = &mainClient.Fake
	for _, reactor := range tc.mainReactors {
		mainClient.AddReactor(reactor.verb, reactor.resource, reactor.reactor(t))
	}
	if tc.bundle != nil {
		for i, res := range tc.bundle.Spec.Resources {
			if res.Spec.Object != nil {
				resUnstr, err := util.RuntimeToUnstructured(res.Spec.Object)
				require.NoError(t, err)
				tc.bundle.Spec.Resources[i].Spec.Object = resUnstr
			}
		}
		tc.smithClientObjects = append(tc.smithClientObjects, tc.bundle)
	}
	bundleClient := smithFake.NewSimpleClientset(tc.smithClientObjects...)
	tc.bundleFake = &bundleClient.Fake
	for _, reactor := range tc.smithReactors {
		bundleClient.AddReactor(reactor.verb, reactor.resource, reactor.reactor(t))
	}
	crdClient := crdFake.NewSimpleClientset(tc.crdClientObjects...)
	tc.crdFake = &crdClient.Fake
	for _, reactor := range tc.crdReactors {
		crdClient.AddReactor(reactor.verb, reactor.resource, reactor.reactor(t))
	}
	var scClient scClientset.Interface
	if tc.enableServiceCatalog {
		scClientFake := scFake.NewSimpleClientset(tc.scClientObjects...)
		tc.scFake = &scClientFake.Fake
		scClient = scClientFake
		for _, reactor := range tc.scReactors {
			scClientFake.AddReactor(reactor.verb, reactor.resource, reactor.reactor(t))
		}
	}

	crdInf := apiext_v1b1inf.NewCustomResourceDefinitionInformer(crdClient, 0, cache.Indexers{})
	bundleInf := client.BundleInformer(bundleClient.SmithV1(), meta_v1.NamespaceAll, 0)
	scheme, err := apitypes.FullScheme(tc.enableServiceCatalog)

	for _, object := range tc.crdClientObjects {
		crd := object.(*apiext_v1b1.CustomResourceDefinition)
		scheme.AddKnownTypeWithName(schema.GroupVersionKind{
			Group:   crd.Spec.Group,
			Version: crd.Spec.Version,
			Kind:    crd.Spec.Names.Kind,
			// obj: unstructured.Unstructured
			// is here _only_ to keep rest scheme happy, we do not currently use scheme to deserialize
		}, &unstructured.Unstructured{})
	}

	multiStore := store.NewMulti()

	bs, err := store.NewBundle(bundleInf, multiStore, nil)
	require.NoError(t, err)
	resourceInfs := apitypes.ResourceInformers(mainClient, scClient, meta_v1.NamespaceAll, 0, true)

	crdStore, err := store.NewCrd(crdInf)
	require.NoError(t, err)

	// Ready Checker
	readyTypes := []map[schema.GroupKind]readychecker.IsObjectReady{ready_types.MainKnownTypes}
	if tc.enableServiceCatalog {
		readyTypes = append(readyTypes, ready_types.ServiceCatalogKnownTypes)
	}
	rc := readychecker.New(crdStore, readyTypes...)

	// Object cleanup
	cleanupTypes := []map[schema.GroupKind]cleanup.SpecCleanup{clean_types.MainKnownTypes}
	if tc.enableServiceCatalog {
		cleanupTypes = append(cleanupTypes, clean_types.ServiceCatalogKnownTypes)
	}
	oc := cleanup.New(cleanupTypes...)

	// Spec check
	specCheck := &speccheck.SpecCheck{
		Scheme:  scheme,
		Cleaner: oc,
	}

	// Look at k8s.io/kubernetes/pkg/controller/garbagecollector/garbagecollector_test.go for inspiration
	srv, clientConfig := testServerAndClientConfig(&tc.testHandler)
	defer srv.Close()

	restMapper := restMapperFromScheme(scheme)

	sc := &smart.DynamicClient{
		CoreDynamic: dynamic.NewClientPool(clientConfig, restMapper, dynamic.LegacyAPIPathResolverFunc),
		Mapper:      restMapper,
	}
	if tc.enableServiceCatalog {
		sc.ScDynamic = sc.CoreDynamic
		sc.ScMapper = restMapper
	}

	stgr := stager.New()
	defer stgr.Shutdown()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	pluginContainers := make(map[smith_v1.PluginName]plugin.PluginContainer, len(tc.plugins))
	for name, factory := range tc.plugins {
		pluginInstance := factory(t)
		pluginContainers[name], err = plugin.NewPluginContainer(
			func() (plugin.Plugin, error) { return pluginInstance, nil })
		require.NoError(t, err)
		tc.pluginsConstructed = append(tc.pluginsConstructed, pluginInstance)
	}

	cntrlr := &BundleController{
		BundleInf:        bundleInf,
		BundleClient:     bundleClient.SmithV1(),
		BundleStore:      bs,
		SmartClient:      sc,
		Rc:               rc,
		Store:            multiStore,
		SpecCheck:        specCheck,
		Queue:            workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "bundle"),
		Workers:          2,
		Namespace:        tc.namespace,
		PluginContainers: pluginContainers,
		Scheme:           scheme,
	}
	cntrlr.Prepare(ctx, crdInf, resourceInfs)

	resourceInfs[apiext_v1b1.SchemeGroupVersion.WithKind("CustomResourceDefinition")] = crdInf
	resourceInfs[smith_v1.BundleGVK] = bundleInf
	infs := make([]cache.InformerSynced, 0, len(resourceInfs))
	stage := stgr.NextStage()
	for gvk, inf := range resourceInfs {
		multiStore.AddInformer(gvk, inf)
		stage.StartWithChannel(inf.Run)
		infs = append(infs, inf.HasSynced)
	}
	require.True(t, cache.WaitForCacheSync(ctx.Done(), infs...))

	if tc.test == nil {
		tc.defaultTest(t, ctx, cntrlr)
	} else {
		tc.test(t, ctx, cntrlr, tc)
	}
}

func (tc *testCase) defaultTest(t *testing.T, ctx context.Context, cntrlr *BundleController) {
	require.NotNil(t, tc.bundle)
	key, err := cache.MetaNamespaceKeyFunc(tc.bundle)
	require.NoError(t, err)
	_, err = cntrlr.processKey(key)
	require.NoError(t, err)
}

func (tc *testCase) verifyActions(t *testing.T) {
	actualActions := sets.NewString()
	for _, actualAction := range tc.testHandler.actions {
		actualActions.Insert(actualAction.String())
	}
	missingActions := tc.expectedActions.Difference(actualActions)
	unexpectedActions := actualActions.Difference(tc.expectedActions)
	assert.Empty(t, missingActions, "expected but was not observed")
	assert.Empty(t, unexpectedActions, "observed but was not expected")
}

func (tc *testCase) verifyPlugins(t *testing.T) {
	invokedPlugins := sets.NewString()
	for _, constructedPlugin := range tc.pluginsConstructed {
		if constructedPlugin.WasInvoked() {
			invokedPlugins.Insert(string(constructedPlugin.Describe().Name))
		}
	}
	missingPlugins := tc.pluginsShouldBeInvoked.Difference(invokedPlugins)
	unexpectedInvocations := invokedPlugins.Difference(tc.pluginsShouldBeInvoked)
	assert.Empty(t, missingPlugins, "expected but was not observed")
	assert.Empty(t, unexpectedInvocations, "observed but was not expected")
}

// testServerAndClientConfig returns a server that listens and a config that can reference it
func testServerAndClientConfig(handler http.Handler) (*httptest.Server, *rest.Config) {
	srv := httptest.NewServer(handler)
	config := &rest.Config{
		Host: srv.URL,
	}
	return srv, config
}

func restMapperFromScheme(scheme *runtime.Scheme) meta.RESTMapper {
	rm := meta.NewDefaultRESTMapper(nil, meta.InterfacesForUnstructured)
	for gvk := range scheme.AllKnownTypes() {
		rm.Add(gvk, meta.RESTScopeNamespace)
	}
	return rm
}

// fakeAction records information about requests to aid in testing.
type fakeAction struct {
	method string
	path   string
	query  string
}

// String returns method=path to aid in testing
func (f *fakeAction) String() string {
	if strings.Contains(f.query, "watch=true") {
		return strings.Join([]string{f.method, f.path, "watch"}, "=")
	}
	if f.query != "" {
		return strings.Join([]string{f.method, f.path, f.query}, "=")
	}
	return strings.Join([]string{f.method, f.path}, "=")
}

type fakeResponse struct {
	statusCode int
	content    []byte
}

type path struct {
	method string
	path   string
	watch  bool
}

// fakeActionHandler holds a list of fakeActions received
type fakeActionHandler struct {
	// statusCode and content returned by this handler for different method + path.
	response map[path]fakeResponse

	lock    sync.Mutex
	actions []fakeAction
}

// ServeHTTP logs the action that occurred and always returns the associated status code
func (f *fakeActionHandler) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	f.lock.Lock()
	defer f.lock.Unlock()

	f.actions = append(f.actions, fakeAction{method: request.Method, path: request.URL.Path, query: request.URL.RawQuery})
	key := path{method: request.Method, path: request.URL.Path, watch: strings.Contains(request.URL.RawQuery, "watch=true")}
	fakeResp, ok := f.response[key]
	if !ok {
		fakeResp = fakeResponse{
			statusCode: http.StatusOK,
			content:    []byte(`{"kind": "List", "items": []}`),
		}
	}
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(fakeResp.statusCode)
	response.Write(fakeResp.content)
}

func SleeperCrdWithStatus() *apiext_v1b1.CustomResourceDefinition {
	crd := sleeper.SleeperCrd()
	crd.Status = apiext_v1b1.CustomResourceDefinitionStatus{
		Conditions: []apiext_v1b1.CustomResourceDefinitionCondition{
			{Type: apiext_v1b1.Established, Status: apiext_v1b1.ConditionTrue},
			{Type: apiext_v1b1.NamesAccepted, Status: apiext_v1b1.ConditionTrue},
		},
	}
	return crd
}

type testingPlugin interface {
	plugin.Plugin
	WasInvoked() bool
}

func configMapWithDependenciesPlugin(t *testing.T) testingPlugin {
	return &configMapWithDeps{
		t: t,
	}
}

type configMapWithDeps struct {
	t          *testing.T
	wasInvoked bool
}

func (p *configMapWithDeps) WasInvoked() bool {
	return p.wasInvoked
}

func (p *configMapWithDeps) Describe() *plugin.Description {
	return &plugin.Description{
		Name: pluginConfigMapWithDeps,
		GVK:  core_v1.SchemeGroupVersion.WithKind("ConfigMap"),
		SpecSchema: []byte(`{
			"type": "object",
			"properties": {
				"p1": {
					"type": "string"
				},
				"p2": {
					"type": "string"
				}
			}
		}`),
	}
}

func (p *configMapWithDeps) Process(pluginSpec map[string]interface{}, context *plugin.Context) (*plugin.ProcessResult, error) {
	p.wasInvoked = true
	failed := p.t.Failed()
	actualShouldExist, _ := pluginSpec["actualShouldExist"].(bool)
	delete(pluginSpec, "actualShouldExist")
	assert.Equal(p.t, map[string]interface{}{"p1": "v1", "p2": sb1}, pluginSpec)

	if actualShouldExist {
		assert.IsType(p.t, &core_v1.ConfigMap{}, context.Actual)
	} else {
		assert.Nil(p.t, context.Actual)
	}

	assert.Len(p.t, context.Dependencies, 1)
	bindingDep, ok := context.Dependencies[resSb1]
	if assert.True(p.t, ok) {
		// Actual
		if assert.IsType(p.t, &sc_v1b1.ServiceBinding{}, bindingDep.Actual) {
			b := bindingDep.Actual.(*sc_v1b1.ServiceBinding)
			assert.Equal(p.t, sb1, b.Name)
			assert.Equal(p.t, testNamespace, b.Namespace)
			assert.Equal(p.t, sc_v1b1.SchemeGroupVersion.WithKind("ServiceBinding"), b.GroupVersionKind())
		}
		// Outputs
		if assert.Len(p.t, bindingDep.Outputs, 1) {
			secret := bindingDep.Outputs[0]
			if assert.IsType(p.t, &core_v1.Secret{}, secret) {
				s := secret.(*core_v1.Secret)
				assert.Equal(p.t, "s1", s.Name)
				assert.Equal(p.t, testNamespace, s.Namespace)
				assert.Equal(p.t, core_v1.SchemeGroupVersion.WithKind("Secret"), s.GroupVersionKind())
			}
		}
		// Aux
		if assert.Len(p.t, bindingDep.Auxiliary, 1) {
			svcInst := bindingDep.Auxiliary[0]
			if assert.IsType(p.t, &sc_v1b1.ServiceInstance{}, svcInst) {
				inst := svcInst.(*sc_v1b1.ServiceInstance)
				assert.Equal(p.t, "si1", inst.Name)
				assert.Equal(p.t, testNamespace, inst.Namespace)
				assert.Equal(p.t, sc_v1b1.SchemeGroupVersion.WithKind("ServiceInstance"), inst.GroupVersionKind())
			}
		}
	}

	if !failed && p.t.Failed() { // one of the assertions failed and it was the first failure in the test
		return nil, errors.New("plugin failed BOOM!")
	}

	return &plugin.ProcessResult{
		Object: &core_v1.ConfigMap{
			TypeMeta: meta_v1.TypeMeta{
				Kind:       "ConfigMap",
				APIVersion: core_v1.SchemeGroupVersion.String(),
			},
		},
	}, nil
}

func simpleConfigMapPlugin(t *testing.T) testingPlugin {
	return &simpleConfigMap{
		t: t,
	}
}

type simpleConfigMap struct {
	t          *testing.T
	wasInvoked bool
}

func (p *simpleConfigMap) WasInvoked() bool {
	return p.wasInvoked
}

func (p *simpleConfigMap) Describe() *plugin.Description {
	return &plugin.Description{
		Name: pluginSimpleConfigMap,
		GVK:  core_v1.SchemeGroupVersion.WithKind("ConfigMap"),
	}
}

func (p *simpleConfigMap) Process(pluginSpec map[string]interface{}, context *plugin.Context) (*plugin.ProcessResult, error) {
	p.wasInvoked = true
	failed := p.t.Failed()
	actualShouldExist, _ := pluginSpec["actualShouldExist"].(bool)

	if actualShouldExist {
		assert.IsType(p.t, &core_v1.ConfigMap{}, context.Actual)
	} else {
		assert.Nil(p.t, context.Actual)
	}

	if !failed && p.t.Failed() { // one of the assertions failed and it was the first failure in the test
		return nil, errors.New("plugin failed BOOM!")
	}

	return &plugin.ProcessResult{
		Object: &core_v1.ConfigMap{
			TypeMeta: meta_v1.TypeMeta{
				Kind:       "ConfigMap",
				APIVersion: core_v1.SchemeGroupVersion.String(),
			},
		},
	}, nil
}
