package bundlec_test

import (
	"context"
	"testing"
	"time"

	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/smith/pkg/controller/bundlec"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
)

// Should postpone the deletion of the controlled object
// if deletion delay hasn't expired yet
func TestDelayDeletePostpone(t *testing.T) {
	t.Parallel()
	m1 := configMapNeedsUpdate()
	if m1.Annotations == nil {
		m1.Annotations = make(map[string]string)
	}
	m1.Annotations["smith.atlassian.com/deletionDelay"] = "1h"
	m1.Annotations["smith.atlassian.com/deletionTimestamp"] = time.Now().UTC().Format(time.RFC3339)
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
		expectedActions: sets.NewString(), // No actions
		appName:         testAppName,
		namespace:       meta_v1.NamespaceAll,
		test: func(t *testing.T, ctx context.Context, cntrlr *bundlec.Controller, tc *testCase) {
			tc.defaultTest(t, ctx, cntrlr)
			tc.assertObjectsToBeDeleted(t, m1)
		},
	}
	tc.run(t)
}
