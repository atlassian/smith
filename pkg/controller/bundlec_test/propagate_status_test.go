package bundlec_test

import (
	"context"
	"testing"

	cond_v1 "github.com/atlassian/ctrl/apis/condition/v1"
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/smith/pkg/controller/bundlec"
	smith_testing "github.com/atlassian/smith/pkg/util/testing"
	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	"github.com/stretchr/testify/require"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// Should correctly propagate status information
func TestStatusPropagated(t *testing.T) {
	t.Parallel()

	si := serviceInstance(false, false, false)
	si.Status.Conditions = append(si.Status.Conditions, sc_v1b1.ServiceInstanceCondition{
		Type:    sc_v1b1.ServiceInstanceConditionReady,
		Status:  sc_v1b1.ConditionFalse,
		Reason:  "ProvisionCallFailed",
		Message: "Error provisioning ServiceInstance of ClusterServiceClass failed",
	})

	tc := testCase{
		scClientObjects:      []runtime.Object{si},
		appName:              testAppName,
		namespace:            testNamespace,
		enableServiceCatalog: true,
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
				},
			},
		},
		test: func(t *testing.T, ctx context.Context, cntrlr *bundlec.Controller, tc *testCase) {
			require.NotNil(t, tc.bundle)
			_, err := cntrlr.ProcessBundle(tc.logger, tc.bundle)
			require.EqualError(t, err, "error processing resource(s): [\"resSi1\"]")
			bundle := tc.findBundleUpdate(t, true)
			smith_testing.AssertResourceCondition(t, bundle, "resSi1", smith_v1.BundleError, cond_v1.ConditionTrue)
			smith_testing.AssertResourceConditionMessage(t, bundle, "resSi1", smith_v1.BundleError, "readiness check failed: Error provisioning ServiceInstance of ClusterServiceClass failed")
		},
	}

	tc.run(t)
}
