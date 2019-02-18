package bundlec_test

import (
	"context"
	"testing"

	cond_v1 "github.com/atlassian/ctrl/apis/condition/v1"
	"github.com/atlassian/smith"
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/smith/pkg/controller/bundlec"
	smith_testing "github.com/atlassian/smith/pkg/util/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	core_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kube_testing "k8s.io/client-go/testing"
)

// Should detect prohibited annotations in resource spec and return error
func TestProhibitedAnnotationsObjectRejected(t *testing.T) {
	t.Parallel()

	r1 := smith_v1.ResourceName("resource1")
	cm1 := &core_v1.ConfigMap{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: core_v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: mapNeedsAnUpdate,
			Annotations: map[string]string{
				smith.DeletionTimestampAnnotation: "2006-01-02T15:04:05Z07:00",
			},
		},
		Data: map[string]string{
			"delete": "this key",
		},
	}

	tc := testCase{
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
						Name: r1,
						Spec: smith_v1.ResourceSpec{
							Object: cm1,
						},
					},
				},
			},
		},
		appName:   testAppName,
		namespace: testNamespace,
		test: func(t *testing.T, ctx context.Context, cntrlr *bundlec.Controller, tc *testCase) {
			external, retriable, err := cntrlr.ProcessBundle(tc.logger, tc.bundle)
			assert.EqualError(t, err, `error processing resource(s): ["`+string(r1)+`"]`)
			assert.True(t, external, "error should be an external error") // user tried to set annotation
			assert.False(t, retriable, "error should not be a retriable error")

			actions := tc.smithFake.Actions()
			require.Len(t, actions, 3)
			bundleUpdate := actions[2].(kube_testing.UpdateAction)
			assert.Equal(t, testNamespace, bundleUpdate.GetNamespace())
			updateBundle := bundleUpdate.GetObject().(*smith_v1.Bundle)
			smith_testing.AssertCondition(t, updateBundle, smith_v1.BundleReady, cond_v1.ConditionFalse)
			smith_testing.AssertCondition(t, updateBundle, smith_v1.BundleInProgress, cond_v1.ConditionFalse)
			smith_testing.AssertCondition(t, updateBundle, smith_v1.BundleError, cond_v1.ConditionTrue)

			smith_testing.AssertResourceCondition(t, updateBundle, r1, smith_v1.ResourceBlocked, cond_v1.ConditionFalse)
			smith_testing.AssertResourceCondition(t, updateBundle, r1, smith_v1.ResourceInProgress, cond_v1.ConditionFalse)
			smith_testing.AssertResourceCondition(t, updateBundle, r1, smith_v1.ResourceReady, cond_v1.ConditionFalse)
			smith_testing.AssertResourceCondition(t, updateBundle, r1, smith_v1.ResourceError, cond_v1.ConditionTrue)

			smith_testing.AssertResourceConditionMessage(t, updateBundle, r1, smith_v1.ResourceError, `annotation "smith.atlassian.com/deletionTimestamp" cannot be set by the user`)
		},
	}
	tc.run(t)
}
