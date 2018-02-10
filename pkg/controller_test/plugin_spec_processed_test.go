package controller_test

import (
	"net/http"
	"testing"

	"github.com/atlassian/smith"
	sleeper_v1 "github.com/atlassian/smith/examples/sleeper/pkg/apis/sleeper/v1"
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"

	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	core_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
)

// Should process plugin specification
func TestPluginSpecProcessed(t *testing.T) {
	t.Parallel()
	tr := true
	tc := testCase{
		crdClientObjects: []runtime.Object{
			SleeperCrdWithStatus(),
		},
		mainClientObjects: []runtime.Object{
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
				},
				Data: map[string][]byte{
					"data": []byte("bla"),
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
				Name:      bundle1,
				Namespace: testNamespace,
				UID:       bundle1uid,
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
							},
						},
					},
					{
						Name:      resSb1,
						DependsOn: []smith_v1.ResourceName{resSi1},
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
						Name: resSleeper1,
						Spec: smith_v1.ResourceSpec{
							Object: &sleeper_v1.Sleeper{
								TypeMeta: meta_v1.TypeMeta{
									Kind:       sleeper_v1.SleeperResourceKind,
									APIVersion: sleeper_v1.SchemeGroupVersion.String(),
								},
								ObjectMeta: meta_v1.ObjectMeta{
									Name: sleeper1,
								},
							}},
					},
					{
						Name:      resP1,
						DependsOn: []smith_v1.ResourceName{resSb1, resSleeper1},
						Spec: smith_v1.ResourceSpec{
							Plugin: &smith_v1.PluginSpec{
								Name:       pluginConfigMapWithDeps,
								ObjectName: m1,
								Spec: map[string]interface{}{
									"p1": "v1", "p2": "{{" + resSb1 + "#metadata.name}}",
								},
							},
						},
					},
				},
			},
		},
		namespace: testNamespace,
		expectedActions: sets.NewString(
			"POST=/api/v1/namespaces/"+testNamespace+"/configmaps",
			"GET=/apis/"+sleeper_v1.SleeperResourceGroupVersion+"/namespaces/"+testNamespace+"/"+sleeper_v1.SleeperResourcePlural+
				"=limit=500&resourceVersion=0",
			"GET=/apis/"+sleeper_v1.SleeperResourceGroupVersion+"/namespaces/"+testNamespace+"/"+sleeper_v1.SleeperResourcePlural+
				"=watch",
			"POST=/apis/"+sleeper_v1.SleeperResourceGroupVersion+"/namespaces/"+testNamespace+"/"+sleeper_v1.SleeperResourcePlural,
		),
		enableServiceCatalog: true,
		testHandler: fakeActionHandler{
			response: map[path]fakeResponse{
				{
					method: "POST",
					path:   "/api/v1/namespaces/" + testNamespace + "/configmaps",
				}: {
					statusCode: http.StatusCreated,
					content: []byte(`{
							"apiVersion": "v1",
							"kind": "ConfigMap",
							"metadata": {
								"name": "` + m1 + `",
								"namespace": "` + testNamespace + `",
								"uid": "` + mapNeedsAnUpdateUid + `",
								"labels": {
									"` + smith.BundleNameLabel + `": "` + bundle1 + `"
								},
								"ownerReferences": [
									{
										"apiVersion": "` + smith_v1.BundleResourceGroupVersion + `",
										"kind": "` + smith_v1.BundleResourceKind + `",
										"name": "` + bundle1 + `",
										"uid": "` + bundle1uid + `",
										"controller": true,
										"blockOwnerDeletion": true
									},
									{
										"apiVersion": "` + sc_v1b1.SchemeGroupVersion.String() + `",
										"kind": "ServiceBinding",
										"name": "` + sb1 + `",
										"uid": "` + sb1uid + `",
										"blockOwnerDeletion": true
									},
									{
										"apiVersion": "` + sleeper_v1.SleeperResourceGroupVersion + `",
										"kind": "` + sleeper_v1.SleeperResourceKind + `",
										"name": "` + sleeper1 + `",
										"uid": "` + sleeper1uid + `",
										"blockOwnerDeletion": true
									}
								] }
							}`),
				},
				{
					method: "GET",
					path:   "/apis/" + sleeper_v1.SleeperResourceGroupVersion + "/namespaces/" + testNamespace + "/" + sleeper_v1.SleeperResourcePlural,
				}: {
					statusCode: http.StatusOK,
					content:    []byte(`{"kind": "List", "items": []}`),
				},
				{
					method: "GET",
					path:   "/apis/" + sleeper_v1.SleeperResourceGroupVersion + "/namespaces/" + testNamespace + "/" + sleeper_v1.SleeperResourcePlural,
					watch:  true,
				}: {
					statusCode: http.StatusOK,
					content:    []byte(`{"type": "ADDED", "object": { "kind": "Sleeper" } }`),
				},
				{
					method: "POST",
					path:   "/apis/crd.atlassian.com/v1/namespaces/" + testNamespace + "/" + sleeper_v1.SleeperResourcePlural,
				}: {
					statusCode: http.StatusCreated,
					content: []byte(`{
							"apiVersion": "crd.atlassian.com/v1",
							"kind": "Sleeper",
							"metadata": {
								"name": "` + sleeper1 + `",
								"namespace": "` + testNamespace + `",
								"uid": "` + sleeper1uid + `",
								"labels": {
									"` + smith.BundleNameLabel + `": "` + bundle1 + `"
								},
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
							"spec": {
								"sleepFor":0,
								"wakeupMessage":""
							},
							"status": {
								"state": "Awake!"
							}
						}`),
				},
			},
		},
		plugins: map[smith_v1.PluginName]func(*testing.T) testingPlugin{
			pluginConfigMapWithDeps: configMapWithDependenciesPlugin(true, true),
		},
		pluginsShouldBeInvoked: sets.NewString(pluginConfigMapWithDeps),
	}
	tc.run(t)
}
