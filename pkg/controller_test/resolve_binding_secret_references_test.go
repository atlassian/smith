package controller_test

import (
	"testing"

	"github.com/atlassian/smith"
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"

	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	core_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// Should resolve ServiceBinding Secret references
func TestResolveBindingSecretReferences(t *testing.T) {
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
					"mysecret": []byte("bla"),
				},
				Type: core_v1.SecretTypeOpaque,
			},
		},
		scClientObjects: []runtime.Object{
			serviceInstance(true, false, false),
			serviceBinding(true, false, false),
			&sc_v1b1.ServiceInstance{
				TypeMeta: meta_v1.TypeMeta{
					Kind:       "ServiceInstance",
					APIVersion: sc_v1b1.SchemeGroupVersion.String(),
				},
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      si2,
					Namespace: testNamespace,
					UID:       si2uid,
					Labels: map[string]string{
						smith.BundleNameLabel: bundle1,
					},
					Annotations: map[string]string{
						"Secret": "bla",
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
						{
							APIVersion:         sc_v1b1.SchemeGroupVersion.String(),
							Kind:               "ServiceBinding",
							Name:               sb1,
							UID:                sb1uid,
							BlockOwnerDeletion: &tr,
						},
					},
				},
				Status: sc_v1b1.ServiceInstanceStatus{
					Conditions: []sc_v1b1.ServiceInstanceCondition{
						{
							Type:   sc_v1b1.ServiceInstanceConditionReady,
							Status: sc_v1b1.ConditionTrue,
						},
					},
				},
			},
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
						Name: resSb1,
						DependsOn: []smith_v1.ResourceName{
							resSi1,
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
						Name: resSi2,
						DependsOn: []smith_v1.ResourceName{
							resSb1,
						},
						Spec: smith_v1.ResourceSpec{
							Object: &sc_v1b1.ServiceInstance{
								TypeMeta: meta_v1.TypeMeta{
									Kind:       "ServiceInstance",
									APIVersion: sc_v1b1.SchemeGroupVersion.String(),
								},
								ObjectMeta: meta_v1.ObjectMeta{
									Name: si2,
									Annotations: map[string]string{
										"Secret": "{{" + resSb1 + ":bindsecret#Data.mysecret}}",
									},
								},
							},
						},
					},
				},
			},
		},
		namespace:            testNamespace,
		enableServiceCatalog: true,
	}
	tc.run(t)
}
