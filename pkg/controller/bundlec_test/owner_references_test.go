package bundlec_test

import (
	"net/http"
	"strconv"
	"testing"

	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/smith/pkg/controller/bundlec"
	core_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
)

// Should construct correct owner references
func TestOwnerReferencesFromReferences(t *testing.T) {
	t.Parallel()
	tc := testCase{
		mainClientObjects: []runtime.Object{
			configMapNeedsUpdate(),
			configMapNeedsUpdateIndex(2),
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
					{
						Name: resMapNeedsAnUpdate + "2",
						References: []smith_v1.Reference{
							// Two ordering refs to the same resource. Should be ok.
							{Resource: resMapNeedsAnUpdate},
							{Resource: resMapNeedsAnUpdate},
						},
						Spec: smith_v1.ResourceSpec{
							Object: &core_v1.ConfigMap{
								TypeMeta: meta_v1.TypeMeta{
									Kind:       "ConfigMap",
									APIVersion: core_v1.SchemeGroupVersion.String(),
								},
								ObjectMeta: meta_v1.ObjectMeta{
									Name: mapNeedsAnUpdate + "2",
								},
							},
						},
					},
				},
			},
		},
		appName:   testAppName,
		namespace: testNamespace,
		expectedActions: sets.NewString(
			"PUT=/api/v1/namespaces/"+testNamespace+"/configmaps/"+mapNeedsAnUpdate,
			"PUT=/api/v1/namespaces/"+testNamespace+"/configmaps/"+mapNeedsAnUpdate+"2",
		),
		testHandler: fakeActionHandler{
			response: map[path]fakeResponse{
				{
					method: "PUT",
					path:   "/api/v1/namespaces/" + testNamespace + "/configmaps/" + mapNeedsAnUpdate,
				}: configMapNeedsUpdateResponse(bundle1, bundle1uid),
				{
					method: "PUT",
					path:   "/api/v1/namespaces/" + testNamespace + "/configmaps/" + mapNeedsAnUpdate + "2",
				}: configMapNeedsUpdateRefsResponse(bundle1, bundle1uid),
			},
		},
	}
	tc.run(t)
}

func configMapNeedsUpdateIndex(index int) *core_v1.ConfigMap {
	tr := true
	return &core_v1.ConfigMap{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: core_v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      mapNeedsAnUpdate + strconv.Itoa(index),
			Namespace: testNamespace,
			UID:       mapNeedsAnUpdateUid + types.UID(strconv.Itoa(index)),
			OwnerReferences: []meta_v1.OwnerReference{
				{
					APIVersion: smith_v1.BundleResourceGroupVersion,
					Kind:       smith_v1.BundleResourceKind,
					Name:       bundle1,
					UID:        bundle1uid,
					Controller: &tr,
					//BlockOwnerDeletion is missing - needs an update
				},
			},
		},
		Data: map[string]string{
			"delete": "this key",
		},
	}
}

func configMapNeedsUpdateRefsResponse(bundleName string, bundleUid types.UID) fakeResponse {
	return fakeResponse{
		statusCode: http.StatusOK,
		content: []byte(`{
							"apiVersion": "v1",
							"kind": "ConfigMap",
							"metadata": {
								"name": "` + mapNeedsAnUpdate + `2",
								"namespace": "` + testNamespace + `",
								"uid": "` + string(mapNeedsAnUpdateUid) + `2",
								"ownerReferences": [
								{
									"apiVersion": "` + smith_v1.BundleResourceGroupVersion + `",
									"kind": "` + smith_v1.BundleResourceKind + `",
									"name": "` + bundleName + `",
									"uid": "` + string(bundleUid) + `",
									"controller": true,
									"blockOwnerDeletion": true
								},
								{
									"apiVersion": "` + core_v1.SchemeGroupVersion.String() + `",
									"kind": "ConfigMap",
									"name": "` + mapNeedsAnUpdate + `",
									"uid": "` + string(mapNeedsAnUpdateUid) + `",
									"blockOwnerDeletion": true
								}
								] }
							}`),
	}
}
