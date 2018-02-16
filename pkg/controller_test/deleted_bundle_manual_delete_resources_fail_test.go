package controller_test

import (
	"context"
	"testing"

	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/smith/pkg/controller"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	core_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	kube_testing "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"
	"net/http"
)

// Should keep the "deleteResources" finalizer if some resources can't be deleted
func TestKeepFinalizerWhenResourceDeletionFails(t *testing.T) {
	t.Parallel()
	now := meta_v1.Now()
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
				Name:              bundle1,
				Namespace:         testNamespace,
				UID:               bundle1uid,
				DeletionTimestamp: &now,
				// Finalizer to enforce manual deletion
				Finalizers:        []string{controller.FinalizerDeleteResources},
			},
			Spec: smith_v1.BundleSpec{
				Resources: []smith_v1.Resource{
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
		expectedActions: sets.NewString(
			"DELETE=/api/v1/namespaces/"+testNamespace+"/configmaps/"+mapNeedsAnUpdate,
			"DELETE=/api/v1/namespaces/"+testNamespace+"/configmaps/"+mapNeedsDelete,
		),
		testHandler: fakeActionHandler{
			response: map[path]fakeResponse{
				{
					method: "DELETE",
					path:   "/api/v1/namespaces/" + testNamespace + "/configmaps/" + mapNeedsAnUpdate,
				}: {
					statusCode: http.StatusOK,
				},
				{
					method: "DELETE",
					path:   "/api/v1/namespaces/" + testNamespace + "/configmaps/" + mapNeedsDelete,
				}: {
					statusCode: http.StatusInternalServerError,
					content:    []byte("something went wrong"),
				},
			},
		},
		namespace:            testNamespace,
		enableServiceCatalog: false,
		test: func(t *testing.T, ctx context.Context, cntrlr *controller.BundleController, tc *testCase, prepare func(ctx context.Context)) {
			prepare(ctx)
			key, err := cache.MetaNamespaceKeyFunc(tc.bundle)
			require.NoError(t, err)
			_, err = cntrlr.ProcessKey(tc.logger, key)
			assert.EqualError(t, err, `an error on the server ("unknown") has prevented the request from succeeding (delete configmaps `+mapNeedsDelete+`)`)

			actions := tc.bundleFake.Actions()
			require.Len(t, actions, 2)
			assert.Implements(t, (*kube_testing.ListAction)(nil), actions[0])
			assert.Implements(t, (*kube_testing.WatchAction)(nil), actions[1])
		},
	}
	tc.run(t)
}
