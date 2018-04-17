package bundlec_test

import (
	"net/http"
	"testing"

	"github.com/atlassian/smith"
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/smith/pkg/controller/bundlec"
	core_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
)

// Should not merge Secret keys
func TestSecretKeysNotMerged(t *testing.T) {
	tr := true
	t.Parallel()
	tc := testCase{
		mainClientObjects: []runtime.Object{
			&core_v1.Secret{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      s1,
					Namespace: testNamespace,
					UID:       s1uid,
					OwnerReferences: []meta_v1.OwnerReference{
						{
							APIVersion:         smith_v1.BundleResourceGroupVersion,
							Kind:               smith_v1.BundleResourceKind,
							Name:               bundle1,
							UID:                bundle1uid,
							Controller:         &tr,
							BlockOwnerDeletion: &tr,
						},
					},
				},
				Data: map[string][]byte{
					"data_actual": []byte("bla0"),
				},
				Type: core_v1.SecretTypeOpaque,
			},
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
							Object: &core_v1.Secret{
								TypeMeta: meta_v1.TypeMeta{
									Kind:       "Secret",
									APIVersion: core_v1.SchemeGroupVersion.String(),
								},
								ObjectMeta: meta_v1.ObjectMeta{
									Name: s1,
								},
								StringData: map[string]string{
									"data_spec2": "bla2",
								},
								Type: core_v1.SecretTypeOpaque,
							},
						},
					},
				},
			},
		},
		namespace:       testNamespace,
		expectedActions: sets.NewString("PUT=/api/v1/namespaces/" + testNamespace + "/secrets/" + s1),
		testHandler: fakeActionHandler{
			response: map[path]fakeResponse{
				{
					method: "PUT",
					path:   "/api/v1/namespaces/" + testNamespace + "/secrets/" + s1,
				}: {
					statusCode: http.StatusOK,
					content: []byte(`{
							"apiVersion": "v1",
							"kind": "Secret",
							"metadata": {
								"name": "` + s1 + `",
								"namespace": "` + testNamespace + `",
								"uid": "` + s1uid + `",
								"labels": {
									"` + smith.BundleNameLabel + `": "` + bundle1 + `"
								},
								"creationTimestamp": null,
								"ownerReferences": [
									{
										"apiVersion": "` + smith_v1.BundleResourceGroupVersion + `",
										"kind": "` + smith_v1.BundleResourceKind + `",
										"name": "` + bundle1 + `",
										"uid": "` + bundle1uid + `",
										"controller": true,
										"blockOwnerDeletion": true
									}
								]
							},
							"data": {
								"data_spec2": "YmxhMg=="
							},
							"type": "Opaque"
						}`),
				},
			},
		},
	}
	tc.run(t)
}
