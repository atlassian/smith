package resources

import (
	"context"
	"log"
	"reflect"
	"time"

	"github.com/atlassian/smith/pkg/apis/smith"
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/smith/pkg/util"
	"github.com/pkg/errors"

	apiext_v1b1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	crdClientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apiext_lst_v1b1 "k8s.io/apiextensions-apiserver/pkg/client/listers/apiextensions/v1beta1"
	api_errors "k8s.io/apimachinery/pkg/api/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
)

func BundleCrd() *apiext_v1b1.CustomResourceDefinition {
	// Schema is based on:
	// https://github.com/kubernetes/community/blob/master/contributors/design-proposals/architecture/identifiers.md
	// https://github.com/kubernetes/community/blob/master/contributors/devel/api-conventions.md
	// https://github.com/kubernetes/community/blob/master/contributors/design-proposals/architecture/namespaces.md
	// https://github.com/kubernetes/kubernetes/tree/master/api/openapi-spec
	return &apiext_v1b1.CustomResourceDefinition{
		ObjectMeta: meta_v1.ObjectMeta{
			Name: smith_v1.BundleResourceName,
		},
		Spec: apiext_v1b1.CustomResourceDefinitionSpec{
			Group:   smith.GroupName,
			Version: smith_v1.BundleResourceVersion,
			Names: apiext_v1b1.CustomResourceDefinitionNames{
				Plural:   smith_v1.BundleResourcePlural,
				Singular: smith_v1.BundleResourceSingular,
				Kind:     smith_v1.BundleResourceKind,
			},
			Validation: &apiext_v1b1.CustomResourceValidation{
				OpenAPIV3Schema: &apiext_v1b1.JSONSchemaProps{
					Properties: map[string]apiext_v1b1.JSONSchemaProps{
						"spec": {
							Type:     "object",
							Required: []string{"resources"},
							Properties: map[string]apiext_v1b1.JSONSchemaProps{
								"resources": {
									Type: "array",
									Items: &apiext_v1b1.JSONSchemaPropsOrArray{
										Schema: &apiext_v1b1.JSONSchemaProps{
											Ref: strPtr("#/definitions/resource"),
										},
									},
								},
							},
						},
					},
					Definitions: apiext_v1b1.JSONSchemaDefinitions{
						"DNS_SUBDOMAIN": {
							Type:      "string",
							MinLength: int64ptr(1),
							MaxLength: int64ptr(253),
							Pattern:   `^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`,
						},
						"resourceName": {
							Ref:         strPtr("#/definitions/DNS_SUBDOMAIN"),
							Description: "ResourceName is a reference to another Resource in the same bundle",
						},
						"resource": {
							Description: "Resource describes an object that should be provisioned",
							Type:        "object",
							Required:    []string{"name", "spec"},
							Properties: map[string]apiext_v1b1.JSONSchemaProps{
								"name": {
									Ref: strPtr("#/definitions/resourceName"),
								},
								"dependsOn": {
									Type: "array",
									Items: &apiext_v1b1.JSONSchemaPropsOrArray{
										Schema: &apiext_v1b1.JSONSchemaProps{
											Ref: strPtr("#/definitions/resourceName"),
										},
									},
								},
								"spec": {
									Type: "object",
									OneOf: []apiext_v1b1.JSONSchemaProps{
										{
											Required: []string{"object"},
											Properties: map[string]apiext_v1b1.JSONSchemaProps{
												"object": {
													Ref: strPtr("#/definitions/objectSpec"),
												},
											},
										},
										{
											Required: []string{"plugin"},
											Properties: map[string]apiext_v1b1.JSONSchemaProps{
												"plugin": {
													Ref: strPtr("#/definitions/pluginSpec"),
												},
											},
										},
									},
								},
							},
						},
						"objectSpec": {
							Ref:         strPtr("#/definitions/typeMeta"),
							Description: "Schema for a resource that describes an object",
							Type:        "object",
							Required:    []string{"metadata"},
							Properties: map[string]apiext_v1b1.JSONSchemaProps{
								"metadata": {
									Ref: strPtr("#/definitions/objectMeta"),
								},
							},
						},
						"pluginSpec": {
							Description: "Schema for a resource that describes a plugin",
							Type:        "object",
							Required:    []string{"name", "objectName"},
							Properties: map[string]apiext_v1b1.JSONSchemaProps{
								"name": {
									Ref: strPtr("#/definitions/DNS_SUBDOMAIN"),
								},
								"objectName": {
									Ref: strPtr("#/definitions/DNS_SUBDOMAIN"),
								},
								"spec": {
									Type: "object",
								},
							},
						},
						"apiVersion": {
							Type:      "string",
							MinLength: int64ptr(1),
						},
						"kind": {
							Type:      "string",
							MinLength: int64ptr(1),
						},
						"typeMeta": {
							Description: "Schema for TypeMeta",
							Type:        "object",
							Required:    []string{"kind", "apiVersion"},
							Properties: map[string]apiext_v1b1.JSONSchemaProps{
								"kind": {
									Ref: strPtr("#/definitions/kind"),
								},
								"apiVersion": {
									Ref: strPtr("#/definitions/apiVersion"),
								},
							},
						},
						"objectMeta": {
							Description: "Schema for some fields of ObjectMeta",
							Type:        "object",
							Properties: map[string]apiext_v1b1.JSONSchemaProps{
								"name": {
									Ref: strPtr("#/definitions/DNS_SUBDOMAIN"),
								},
								"labels": {
									Type: "object",
									AdditionalProperties: &apiext_v1b1.JSONSchemaPropsOrBool{
										Schema: &apiext_v1b1.JSONSchemaProps{
											Type:      "string",
											MaxLength: int64ptr(63),
											Pattern:   "^(([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9])?$",
										},
									},
								},
								"annotations": {
									Type: "object",
									AdditionalProperties: &apiext_v1b1.JSONSchemaPropsOrBool{
										Schema: &apiext_v1b1.JSONSchemaProps{
											Type: "string",
										},
									},
								},
								"ownerReference": {
									Type:     "object",
									Required: []string{"apiVersion", "kind", "name"},
									Properties: map[string]apiext_v1b1.JSONSchemaProps{
										"kind": {
											Ref: strPtr("#/definitions/kind"),
										},
										"apiVersion": {
											Ref: strPtr("#/definitions/apiVersion"),
										},
										"name": {
											Ref: strPtr("#/definitions/DNS_SUBDOMAIN"),
										},
										"blockOwnerDeletion": {
											Type: "boolean",
										},
										"controller": {
											Type: "boolean",
										},
									},
								},
								"ownerReferences": {
									Type: "array",
									Items: &apiext_v1b1.JSONSchemaPropsOrArray{
										Schema: &apiext_v1b1.JSONSchemaProps{
											Ref: strPtr("#/definitions/ownerReference"),
										},
									},
								},
								"initializer": {
									Type:     "object",
									Required: []string{"name"},
									Properties: map[string]apiext_v1b1.JSONSchemaProps{
										"name": {
											Type: "string",
										},
									},
								},
								"initializers": {
									Type:     "object",
									Required: []string{"pending"},
									Properties: map[string]apiext_v1b1.JSONSchemaProps{
										"pending": {
											Type: "array",
											Items: &apiext_v1b1.JSONSchemaPropsOrArray{
												Schema: &apiext_v1b1.JSONSchemaProps{
													Ref: strPtr("#/definitions/initializer"),
												},
											},
										},
									},
								},
								"finalizers": {
									Type: "array",
									Items: &apiext_v1b1.JSONSchemaPropsOrArray{
										Schema: &apiext_v1b1.JSONSchemaProps{
											Type:      "string",
											MinLength: int64ptr(1),
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func strPtr(str string) *string {
	return &str
}

func int64ptr(val int64) *int64 {
	return &val
}

func EnsureCrdExistsAndIsEstablished(ctx context.Context, defaulter runtime.ObjectDefaulter, clientset crdClientset.Interface, crdLister apiext_lst_v1b1.CustomResourceDefinitionLister, crd *apiext_v1b1.CustomResourceDefinition) error {
	err := EnsureCrdExists(ctx, defaulter, clientset, crdLister, crd)
	if err != nil {
		return err
	}
	log.Printf("Waiting for CustomResourceDefinition %s to become established", crd.Name)
	return WaitForCrdToBecomeEstablished(ctx, crdLister, crd)
}

func EnsureCrdExists(ctx context.Context, defaulter runtime.ObjectDefaulter, clientset crdClientset.Interface, crdLister apiext_lst_v1b1.CustomResourceDefinitionLister, crd *apiext_v1b1.CustomResourceDefinition) error {
	for {
		obj, err := crdLister.Get(crd.Name)
		notFound := api_errors.IsNotFound(err)
		if err != nil && !notFound {
			return err
		}
		if notFound {
			log.Printf("Creating CustomResourceDefinition %s", crd.Name)
			_, err = clientset.ApiextensionsV1beta1().CustomResourceDefinitions().Create(crd)
			if err == nil {
				log.Printf("CustomResourceDefinition %s created", crd.Name)
				return nil
			}
			if !api_errors.IsAlreadyExists(err) {
				return errors.Wrapf(err, "failed to create %s CustomResourceDefinition", crd.Name)
			}
			log.Printf("CustomResourceDefinition %s was created concurrently", crd.Name)
		} else {
			if IsEqualCrd(crd, obj, defaulter) {
				return nil
			}
			log.Printf("Updating CustomResourceDefinition %s", crd.Name)
			obj = obj.DeepCopy()
			obj.Spec = crd.Spec
			obj.Annotations = crd.Annotations
			obj.Labels = crd.Labels
			// TODO erasing the status is only necessary because there is no support for generation/observedGeneration at the moment
			obj.Status = apiext_v1b1.CustomResourceDefinitionStatus{}
			_, err = clientset.ApiextensionsV1beta1().CustomResourceDefinitions().Update(obj) // This is a CAS
			if err == nil {
				log.Printf("CustomResourceDefinition %s updated", crd.Name)
				return nil
			}
			if !api_errors.IsConflict(err) {
				return errors.Wrapf(err, "failed to update CustomResourceDefinition %s", crd.Name)
			}
			log.Printf("Conflict updating CustomResourceDefinition %s, retrying", crd.Name)
		}
		// wait for store to pick up the object and re-iterate
		if err = util.Sleep(ctx, time.Second); err != nil {
			return err
		}
	}
}

func WaitForCrdToBecomeEstablished(ctx context.Context, crdLister apiext_lst_v1b1.CustomResourceDefinitionLister, crd *apiext_v1b1.CustomResourceDefinition) error {
	return wait.PollUntil(100*time.Millisecond, func() (done bool, err error) {
		obj, err := crdLister.Get(crd.Name)
		if err != nil {
			if api_errors.IsNotFound(err) {
				return false, nil
			}
			return false, err
		}
		// TODO check generation/observedGeneration when supported
		established := false
		for _, cond := range obj.Status.Conditions {
			switch cond.Type {
			case apiext_v1b1.Established:
				if cond.Status == apiext_v1b1.ConditionTrue {
					established = true
				}
			case apiext_v1b1.NamesAccepted:
				if cond.Status == apiext_v1b1.ConditionFalse {
					return false, errors.Errorf("failed to create CRD %s: name conflict: %s", crd.Name, cond.Reason)
				}
			}
		}
		return established, nil
	}, ctx.Done())
}

// IsCrdConditionTrue indicates if the condition is present and strictly true
func IsCrdConditionTrue(crd *apiext_v1b1.CustomResourceDefinition, conditionType apiext_v1b1.CustomResourceDefinitionConditionType) bool {
	return IsCrdConditionPresentAndEqual(crd, conditionType, apiext_v1b1.ConditionTrue)
}

// IsCrdConditionPresentAndEqual indicates if the condition is present and equal to the arg
func IsCrdConditionPresentAndEqual(crd *apiext_v1b1.CustomResourceDefinition, conditionType apiext_v1b1.CustomResourceDefinitionConditionType, status apiext_v1b1.ConditionStatus) bool {
	for _, condition := range crd.Status.Conditions {
		if condition.Type == conditionType {
			return condition.Status == status
		}
	}
	return false
}

func IsEqualCrd(a, b *apiext_v1b1.CustomResourceDefinition, defaulter runtime.ObjectDefaulter) bool {
	aCopy := *a
	bCopy := *b
	a = &aCopy
	b = &bCopy

	defaulter.Default(a)
	defaulter.Default(b)

	// Ignoring labels and annotations for now
	as := a.Spec
	bs := b.Spec
	return as.Group == bs.Group &&
		as.Version == bs.Version &&
		isEqualCrdNames(as.Names, bs.Names) &&
		a.Spec.Scope == b.Spec.Scope &&
		isEqualValidation(as.Validation, bs.Validation)
}

func isEqualCrdNames(n1, n2 apiext_v1b1.CustomResourceDefinitionNames) bool {
	return n1.Plural == n2.Plural &&
		n1.Singular == n2.Singular &&
		isEqualShortNames(n1.ShortNames, n2.ShortNames) &&
		n1.Kind == n2.Kind &&
		n1.ListKind == n2.ListKind
}

func isEqualShortNames(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	for i, v := range a {
		if v != b[i] {
			return false
		}
	}

	return true
}

func isEqualValidation(av, bv *apiext_v1b1.CustomResourceValidation) bool {
	return reflect.DeepEqual(av, bv)
}
