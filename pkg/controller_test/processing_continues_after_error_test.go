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
	core_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	kube_testing "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"
)

// Should continue to process resources if they are not blocked by an error
func TestProcessingContinuesAfterNonBlockingError(t *testing.T) {
	t.Parallel()
	tc := testCase{
		mainClientObjects: []runtime.Object{
			configMapNeedsDelete(),
			configMapNeedsUpdate(),
		},
		scClientObjects: []runtime.Object{
			serviceInstance(false, false, true),
		},
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
						Name: resMapNeedsAnUpdate,
						Spec: smith_v1.ResourceSpec{
							Object: &core_v1.ConfigMap{
								TypeMeta: meta_v1.TypeMeta{
									Kind:       "ConfigMap",
									APIVersion: core_v1.SchemeGroupVersion.String(),
								},
								ObjectMeta: meta_v1.ObjectMeta{
									Name: mapNeedsAnUpdate,
								},
							},
						},
					},
				},
			},
		},
		namespace:            testNamespace,
		expectedActions:      sets.NewString("PUT=/api/v1/namespaces/" + testNamespace + "/configmaps/" + mapNeedsAnUpdate),
		enableServiceCatalog: true,
		testHandler: fakeActionHandler{
			response: map[path]fakeResponse{
				{
					method: "PUT",
					path:   "/api/v1/namespaces/" + testNamespace + "/configmaps/" + mapNeedsAnUpdate,
				}: configMapNeedsUpdateResponse(bundle1, bundle1uid),
			},
		},
		test: func(t *testing.T, ctx context.Context, cntrlr *controller.BundleController, tc *testCase) {
			key, err := cache.MetaNamespaceKeyFunc(tc.bundle)
			require.NoError(t, err)
			retriable, err := cntrlr.ProcessKey(tc.logger, key)
			assert.EqualError(t, err, `error processing resource(s): ["`+resSi1+`"]`)
			assert.False(t, retriable)
			actions := tc.smithFake.Actions()
			require.Len(t, actions, 3)
			bundleUpdate := actions[2].(kube_testing.UpdateAction)
			assert.Equal(t, testNamespace, bundleUpdate.GetNamespace())
			updateBundle := bundleUpdate.GetObject().(*smith_v1.Bundle)
			smith_testing.AssertCondition(t, updateBundle, smith_v1.BundleReady, smith_v1.ConditionFalse)
			smith_testing.AssertCondition(t, updateBundle, smith_v1.BundleInProgress, smith_v1.ConditionFalse)
			smith_testing.AssertCondition(t, updateBundle, smith_v1.BundleError, smith_v1.ConditionTrue)

			smith_testing.AssertResourceCondition(t, updateBundle, resSi1, smith_v1.ResourceBlocked, smith_v1.ConditionFalse)
			smith_testing.AssertResourceCondition(t, updateBundle, resSi1, smith_v1.ResourceInProgress, smith_v1.ConditionFalse)
			smith_testing.AssertResourceCondition(t, updateBundle, resSi1, smith_v1.ResourceReady, smith_v1.ConditionFalse)
			resCond := smith_testing.AssertResourceCondition(t, updateBundle, resSi1, smith_v1.ResourceError, smith_v1.ConditionTrue)
			if resCond != nil {
				assert.Equal(t, smith_v1.ResourceReasonTerminalError, resCond.Reason)
				assert.Equal(t, "readiness check failed: BlaBla: Oh no!", resCond.Message)
			}
			smith_testing.AssertResourceCondition(t, updateBundle, resMapNeedsAnUpdate, smith_v1.ResourceBlocked, smith_v1.ConditionFalse)
			smith_testing.AssertResourceCondition(t, updateBundle, resMapNeedsAnUpdate, smith_v1.ResourceInProgress, smith_v1.ConditionFalse)
			smith_testing.AssertResourceCondition(t, updateBundle, resMapNeedsAnUpdate, smith_v1.ResourceReady, smith_v1.ConditionTrue)
			smith_testing.AssertResourceCondition(t, updateBundle, resMapNeedsAnUpdate, smith_v1.ResourceError, smith_v1.ConditionFalse)
		},
	}
	tc.run(t)
}
