package bundlec_test

import (
	"context"
	"net/http"
	"testing"

	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/smith/pkg/controller/bundlec"
	smith_testing "github.com/atlassian/smith/pkg/util/testing"
	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	"github.com/stretchr/testify/require"
	core_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
)

// Should resolve ServiceBinding Secret references
func TestResolveBindingSecretReferences(t *testing.T) {
	t.Parallel()
	tr := true
	sb1ref := smith_v1.Reference{
		Name:     resSb1 + "-mysecret",
		Resource: smith_v1.ResourceName(resSb1),
		Path:     "Data.mysecret",
		Modifier: smith_v1.ReferenceModifierBindSecret,
	}
	tc := testCase{
		mainClientObjects: []runtime.Object{
			configMapNeedsUpdate(),
			&core_v1.Secret{
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
					Finalizers: []string{bundlec.FinalizerDeleteResources},
				},
				Data: map[string][]byte{
					"mysecret": []byte("bla"),
				},
				Type: core_v1.SecretTypeOpaque,
			},
		},
		scClientObjects: []runtime.Object{
			serviceInstance(true, false, false),
			serviceBinding(true, false, false),
		},
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
					{
						Name: resSb1,
						References: []smith_v1.Reference{
							{Resource: smith_v1.ResourceName(resSi1)},
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
										Name: si1,
									},
									SecretName: s1,
								},
							},
						},
					},
					{
						Name: "map-needs-update",
						References: []smith_v1.Reference{
							sb1ref,
						},
						Spec: smith_v1.ResourceSpec{
							Object: &core_v1.ConfigMap{
								TypeMeta: meta_v1.TypeMeta{
									Kind:       "ConfigMap",
									APIVersion: core_v1.SchemeGroupVersion.String(),
								},
								ObjectMeta: meta_v1.ObjectMeta{
									Name: mapNeedsAnUpdate,
									Annotations: map[string]string{
										"Secret": sb1ref.Ref(),
									},
								},
							},
						},
					},
				},
			},
		},
		appName:         testAppName,
		namespace:       testNamespace,
		expectedActions: sets.NewString("PUT=/api/v1/namespaces/" + testNamespace + "/configmaps/" + mapNeedsAnUpdate),
		testHandler: fakeActionHandler{
			response: map[path]fakeResponse{
				{
					method: "PUT",
					path:   "/api/v1/namespaces/" + testNamespace + "/configmaps/" + mapNeedsAnUpdate,
				}: {
					statusCode: http.StatusOK,
					content: []byte(`{
							"apiVersion": "v1",
							"kind": "ConfigMap",
							"data":{"delete":"this key"},
							"metadata": {
								"name": "` + mapNeedsAnUpdate + `",
								"namespace": "` + testNamespace + `",
								"uid": "` + string(mapNeedsAnUpdateUid) + `",
								"annotations": { "Secret":"bla" },
								"ownerReferences": [
								{
									"apiVersion": "` + smith_v1.BundleResourceGroupVersion + `",
									"kind": "` + smith_v1.BundleResourceKind + `",
									"name": "` + bundle1 + `",
									"uid": "` + string(bundle1uid) + `",
									"controller": true,
									"blockOwnerDeletion": true
								},
								{
									"apiVersion": "` + sc_v1b1.SchemeGroupVersion.String() + `",
									"kind": "ServiceBinding",
									"name": "` + sb1 + `",
									"uid": "` + string(sb1uid) + `",
									"blockOwnerDeletion": true
								}
								] }
							}`),
				},
			},
		}, enableServiceCatalog: true,
		test: func(t *testing.T, ctx context.Context, cntrlr *bundlec.Controller, tc *testCase) {
			tc.defaultTest(t, ctx, cntrlr)
			bundle := tc.findBundleUpdate(t, true)
			require.NotNil(t, bundle, "Bundle update action not found: %v", tc.smithFake.Actions())
			smith_testing.AssertCondition(t, bundle, smith_v1.BundleReady, smith_v1.ConditionTrue)
			smith_testing.AssertCondition(t, bundle, smith_v1.BundleInProgress, smith_v1.ConditionFalse)
			smith_testing.AssertCondition(t, bundle, smith_v1.BundleError, smith_v1.ConditionFalse)

		},
	}
	tc.run(t)
}
