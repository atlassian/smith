package bundlec_test

import (
	"context"
	"net/http"
	"testing"

	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/smith/pkg/controller/bundlec"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
)

// Should delete controlled object that is not in the bundle
func TestDeleteRemovedObject(t *testing.T) {
	t.Parallel()
	m1 := configMapNeedsUpdate()
	m2 := configMapMarkedForDeletion()
	tc := testCase{
		mainClientObjects: []runtime.Object{
			m1,
			m2,
		},
		bundle: &smith_v1.Bundle{
			ObjectMeta: meta_v1.ObjectMeta{
				Name:       bundle1,
				Namespace:  testNamespace,
				UID:        bundle1uid,
				Finalizers: []string{bundlec.FinalizerDeleteResources},
			},
		},
		expectedActions: sets.NewString("DELETE=/api/v1/namespaces/" + testNamespace + "/configmaps/" + mapNeedsAnUpdate),
		appName:         testAppName,
		namespace:       meta_v1.NamespaceAll,
		testHandler: fakeActionHandler{
			response: map[path]fakeResponse{
				{
					method: "DELETE",
					path:   "/api/v1/namespaces/" + testNamespace + "/configmaps/" + mapNeedsAnUpdate,
				}: {
					statusCode: http.StatusOK,
				},
			},
		},
		test: func(t *testing.T, ctx context.Context, cntrlr *bundlec.Controller, tc *testCase) {
			tc.defaultTest(t, ctx, cntrlr)
			tc.assertObjectsToBeDeleted(t, m1, m2)
		},
	}
	tc.run(t)
}
