package controller_test

import (
	"net/http"
	"testing"

	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/smith/pkg/controller"

	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
)

// Should delete controlled object that is not in the bundle
func TestDeleteRemovedObject(t *testing.T) {
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
				Finalizers: []string{controller.FinalizerDeleteResources},
			},
		},
		expectedActions: sets.NewString("DELETE=/api/v1/namespaces/" + testNamespace + "/configmaps/" + mapNeedsAnUpdate),
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
	}
	tc.run(t)
}
