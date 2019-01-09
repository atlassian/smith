package bundlec_test

import (
	"context"
	"testing"

	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/smith/pkg/controller/bundlec"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
)

// Should initiate a delay for deletion controlled object that is not in the bundle
func TestDelayDeleteStart(t *testing.T) {
	t.Parallel()
	m1 := configMapNeedsUpdate()
	if m1.Annotations == nil {
		m1.Annotations = make(map[string]string)
	}
	m1.Annotations["smith.atlassian.com/deletionDelay"] = "1h"
	tc := testCase{
		mainClientObjects: []runtime.Object{
			m1,
		},
		bundle: &smith_v1.Bundle{
			ObjectMeta: meta_v1.ObjectMeta{
				Name:       bundle1,
				Namespace:  testNamespace,
				UID:        bundle1uid,
				Finalizers: []string{bundlec.FinalizerDeleteResources},
			},
		},
		expectedActions: sets.NewString("PUT=/api/v1/namespaces/" + testNamespace + "/configmaps/" + mapNeedsAnUpdate),
		appName:         testAppName,
		namespace:       meta_v1.NamespaceAll,
		testHandler: fakeActionHandler{
			response: map[path]fakeResponse{
				{
					method: "PUT",
					path:   "/api/v1/namespaces/" + testNamespace + "/configmaps/" + mapNeedsAnUpdate,
				}: configMapNeedsUpdateResponse(bundle1, bundle1uid),
			},
		},
		test: func(t *testing.T, ctx context.Context, cntrlr *bundlec.Controller, tc *testCase) {
			tc.defaultTest(t, ctx, cntrlr)
			tc.assertObjectsToBeDeleted(t, m1)
		},
	}
	tc.run(t)
}
