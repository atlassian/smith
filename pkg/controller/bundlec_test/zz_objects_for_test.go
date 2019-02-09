package bundlec_test

import (
	"net/http"

	"github.com/atlassian/smith/examples/sleeper"
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/smith/pkg/specchecker/builtin"
	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	core_v1 "k8s.io/api/core/v1"
	apiext_v1b1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func sleeperCrdWithStatus() *apiext_v1b1.CustomResourceDefinition {
	crd := sleeper.Crd()
	crd.Status = apiext_v1b1.CustomResourceDefinitionStatus{
		Conditions: []apiext_v1b1.CustomResourceDefinitionCondition{
			{Type: apiext_v1b1.Established, Status: apiext_v1b1.ConditionTrue},
			{Type: apiext_v1b1.NamesAccepted, Status: apiext_v1b1.ConditionTrue},
		},
	}
	return crd
}

func serviceInstance(ready, inProgress, error bool) *sc_v1b1.ServiceInstance {
	tr := true
	var status sc_v1b1.ServiceInstanceStatus
	if ready {
		status.Conditions = append(status.Conditions, sc_v1b1.ServiceInstanceCondition{
			Type:   sc_v1b1.ServiceInstanceConditionReady,
			Status: sc_v1b1.ConditionTrue,
		})
	}
	if inProgress {
		status.Conditions = append(status.Conditions, sc_v1b1.ServiceInstanceCondition{
			Type:    sc_v1b1.ServiceInstanceConditionReady,
			Status:  sc_v1b1.ConditionFalse,
			Reason:  "Provisioning",
			Message: "Doing something",
		})
	}
	if error {
		status.Conditions = append(status.Conditions, sc_v1b1.ServiceInstanceCondition{
			Type:    sc_v1b1.ServiceInstanceConditionFailed,
			Status:  sc_v1b1.ConditionTrue,
			Reason:  "BlaBla",
			Message: "Oh no!",
		})
	}
	return &sc_v1b1.ServiceInstance{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "ServiceInstance",
			APIVersion: sc_v1b1.SchemeGroupVersion.String(),
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      si1,
			Namespace: testNamespace,
			UID:       si1uid,
			Annotations: map[string]string{
				builtin.SecretParametersChecksumAnnotation: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			},
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
		Status: status,
		Spec:   serviceInstanceSpec,
	}
}

func serviceBinding(ready, inProgress, error bool) *sc_v1b1.ServiceBinding {
	tr := true
	var status sc_v1b1.ServiceBindingStatus
	if ready {
		status.Conditions = append(status.Conditions, sc_v1b1.ServiceBindingCondition{
			Type:   sc_v1b1.ServiceBindingConditionReady,
			Status: sc_v1b1.ConditionTrue,
		})
	}
	if inProgress {
		status.Conditions = append(status.Conditions, sc_v1b1.ServiceBindingCondition{
			Type:    sc_v1b1.ServiceBindingConditionReady,
			Status:  sc_v1b1.ConditionFalse,
			Reason:  "WorkingOnIt",
			Message: "Doing something",
		})
	}
	if error {
		status.Conditions = append(status.Conditions, sc_v1b1.ServiceBindingCondition{
			Type:    sc_v1b1.ServiceBindingConditionFailed,
			Status:  sc_v1b1.ConditionTrue,
			Reason:  "BlaBla",
			Message: "Oh no!",
		})
	}
	return &sc_v1b1.ServiceBinding{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "ServiceBinding",
			APIVersion: sc_v1b1.SchemeGroupVersion.String(),
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      sb1,
			Namespace: testNamespace,
			UID:       sb1uid,
			OwnerReferences: []meta_v1.OwnerReference{
				{
					APIVersion:         smith_v1.BundleResourceGroupVersion,
					Kind:               smith_v1.BundleResourceKind,
					Name:               bundle1,
					UID:                bundle1uid,
					Controller:         &tr,
					BlockOwnerDeletion: &tr,
				},
				{
					APIVersion:         sc_v1b1.SchemeGroupVersion.String(),
					Kind:               "ServiceInstance",
					Name:               si1,
					UID:                si1uid,
					BlockOwnerDeletion: &tr,
				},
			},
		},
		Spec: sc_v1b1.ServiceBindingSpec{
			InstanceRef: sc_v1b1.LocalObjectReference{
				Name: si1,
			},
			SecretName: s1,
		},
		Status: status,
	}
}

func configMapNeedsUpdate() *core_v1.ConfigMap {
	tr := true
	return &core_v1.ConfigMap{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: core_v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      mapNeedsAnUpdate,
			Namespace: testNamespace,
			UID:       mapNeedsAnUpdateUid,
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

func configMapNeedsUpdateResponse(bundleName string, bundleUid types.UID) fakeResponse {
	return fakeResponse{
		statusCode: http.StatusOK,
		content: []byte(`{
							"apiVersion": "v1",
							"kind": "ConfigMap",
							"metadata": {
								"name": "` + mapNeedsAnUpdate + `",
								"namespace": "` + testNamespace + `",
								"uid": "` + string(mapNeedsAnUpdateUid) + `",
								"ownerReferences": [{
									"apiVersion": "` + smith_v1.BundleResourceGroupVersion + `",
									"kind": "` + smith_v1.BundleResourceKind + `",
									"name": "` + bundleName + `",
									"uid": "` + string(bundleUid) + `",
									"controller": true,
									"blockOwnerDeletion": true
								}] }
							}`),
	}
}

func configMapNeedsDelete() *core_v1.ConfigMap {
	tr := true
	return &core_v1.ConfigMap{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: core_v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      mapNeedsDelete,
			Namespace: testNamespace,
			UID:       mapNeedsDeleteUid,
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
	}
}

func configMapMarkedForDeletion() *core_v1.ConfigMap {
	tr := true
	now := meta_v1.Now()
	return &core_v1.ConfigMap{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: core_v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name:              mapMarkedForDeletion,
			Namespace:         testNamespace,
			UID:               mapMarkedForDeletionUid,
			DeletionTimestamp: &now,
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
	}
}
