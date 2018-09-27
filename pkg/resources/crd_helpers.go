package resources

import (
	"context"
	"encoding/json"
	"io"
	"reflect"
	"time"

	ctrlLogz "github.com/atlassian/ctrl/logz"
	"github.com/atlassian/smith/pkg/apis/smith"
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/smith/pkg/util"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	yaml "gopkg.in/yaml.v2"
	apiext_v1b1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	apiExtClientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apiext_lst_v1b1 "k8s.io/apiextensions-apiserver/pkg/client/listers/apiextensions/v1beta1"
	api_errors "k8s.io/apimachinery/pkg/api/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
)

func PrintCleanedObject(sink io.Writer, format string, obj runtime.Object) error {
	u, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return errors.Wrap(err, "failed to convert objed to unstructured")
	}
	unstructured.RemoveNestedField(u, "metadata", "creationTimestamp")
	unstructured.RemoveNestedField(u, "status")
	switch format {
	case "json":
		enc := json.NewEncoder(sink)
		enc.SetIndent("", "  ")
		enc.SetEscapeHTML(false)
		err := enc.Encode(u)
		return errors.Wrap(err, "failed to marshal into JSON")
	case "yaml":
		data, err := yaml.Marshal(u)
		if err != nil {
			return errors.Wrap(err, "failed to marshal into YAML")
		}
		_, err = sink.Write(data)
		return errors.Wrap(err, "failed to write to sink")
	default:
		return errors.Errorf("unsupported output format %q", format)
	}
}

func BundleCrd() *apiext_v1b1.CustomResourceDefinition {
	// Schema is based on:
	// https://github.com/kubernetes/community/blob/master/contributors/design-proposals/architecture/identifiers.md
	// https://github.com/kubernetes/community/blob/master/contributors/devel/api-conventions.md
	// https://github.com/kubernetes/community/blob/master/contributors/design-proposals/architecture/namespaces.md
	// https://github.com/kubernetes/kubernetes/tree/master/api/openapi-spec

	// definitions are not supported, do what we can :)

	dnsSubdomain := apiext_v1b1.JSONSchemaProps{
		Type:      "string",
		MinLength: int64ptr(1),
		MaxLength: int64ptr(253),
		Pattern:   `^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`,
	}
	referenceName := apiext_v1b1.JSONSchemaProps{
		Type:      "string",
		MinLength: int64ptr(1),
		MaxLength: int64ptr(253),
		Pattern:   `^[a-zA-Z0-9]([-a-zA-Z0-9]*[a-zA-Z0-9])?(\.[a-zA-Z0-9]([-a-zA-Z0-9]*[a-zA-Z0-9])?)*$`,
	}
	resourceName := dnsSubdomain
	apiVersion := apiext_v1b1.JSONSchemaProps{
		Type:      "string",
		MinLength: int64ptr(1),
	}
	kind := apiext_v1b1.JSONSchemaProps{
		Type:      "string",
		MinLength: int64ptr(1),
	}
	ownerReference := apiext_v1b1.JSONSchemaProps{
		Type:     "object",
		Required: []string{"apiVersion", "kind", "name"},
		Properties: map[string]apiext_v1b1.JSONSchemaProps{
			"kind":       kind,
			"apiVersion": apiVersion,
			"name":       dnsSubdomain,
			"blockOwnerDeletion": {
				Type: "boolean",
			},
			"controller": {
				Type: "boolean",
			},
		},
	}
	initializer := apiext_v1b1.JSONSchemaProps{
		Type:     "object",
		Required: []string{"name"},
		Properties: map[string]apiext_v1b1.JSONSchemaProps{
			"name": {
				Type: "string",
			},
		},
	}
	objectMeta := apiext_v1b1.JSONSchemaProps{
		Description: "Schema for some fields of ObjectMeta",
		Type:        "object",
		Properties: map[string]apiext_v1b1.JSONSchemaProps{
			"name": dnsSubdomain,
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
			"ownerReferences": {
				Type: "array",
				Items: &apiext_v1b1.JSONSchemaPropsOrArray{
					Schema: &ownerReference,
				},
			},
			"initializers": {
				Type:     "object",
				Required: []string{"pending"},
				Properties: map[string]apiext_v1b1.JSONSchemaProps{
					"pending": {
						Type: "array",
						Items: &apiext_v1b1.JSONSchemaPropsOrArray{
							Schema: &initializer,
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
	}
	objectSpec := apiext_v1b1.JSONSchemaProps{
		Description: "Schema for a resource that describes an object",
		Type:        "object",
		Required:    []string{"apiVersion", "kind", "metadata"},
		Properties: map[string]apiext_v1b1.JSONSchemaProps{
			"kind":       kind,
			"apiVersion": apiVersion,
			"metadata":   objectMeta,
		},
	}
	pluginSpec := apiext_v1b1.JSONSchemaProps{
		Description: "Schema for a resource that describes a plugin",
		Type:        "object",
		Required:    []string{"name", "objectName"},
		Properties: map[string]apiext_v1b1.JSONSchemaProps{
			"name":       dnsSubdomain,
			"objectName": dnsSubdomain,
			"spec": {
				Type: "object",
			},
		},
	}
	reference := apiext_v1b1.JSONSchemaProps{
		Description: "A reference to a path in another resource",
		Type:        "object",
		Required:    []string{"resource"},
		Properties: map[string]apiext_v1b1.JSONSchemaProps{
			"name":     referenceName,
			"resource": resourceName,
			"example": {
				Description: "Example of how we expect reference to resolve. Used for validation",
			},
			"modifier": dnsSubdomain,
			"path": {
				Description: "JSONPath expression used to extract data from resource",
				Type:        "string",
			},
		},
	}
	resource := apiext_v1b1.JSONSchemaProps{
		Description: "Resource describes an object that should be provisioned",
		Type:        "object",
		Required:    []string{"name", "spec"},
		Properties: map[string]apiext_v1b1.JSONSchemaProps{
			"name": resourceName,
			"references": {
				Type: "array",
				Items: &apiext_v1b1.JSONSchemaPropsOrArray{
					Schema: &reference,
				},
			},
			"spec": {
				Type: "object",
				OneOf: []apiext_v1b1.JSONSchemaProps{
					{
						Required: []string{"object"},
						Properties: map[string]apiext_v1b1.JSONSchemaProps{
							"object": objectSpec,
						},
					},
					{
						Required: []string{"plugin"},
						Properties: map[string]apiext_v1b1.JSONSchemaProps{
							"plugin": pluginSpec,
						},
					},
				},
			},
		},
	}
	bundleSpec := apiext_v1b1.JSONSchemaProps{
		Type:     "object",
		Required: []string{"resources"},
		Properties: map[string]apiext_v1b1.JSONSchemaProps{
			"resources": {
				Type: "array",
				Items: &apiext_v1b1.JSONSchemaPropsOrArray{
					Schema: &resource,
				},
			},
		},
	}
	condition := apiext_v1b1.JSONSchemaProps{
		Type:     "object",
		Required: []string{"type", "status"},
		Properties: map[string]apiext_v1b1.JSONSchemaProps{
			"type": {
				Type:      "string",
				MinLength: int64ptr(1),
			},
			"status": {
				Type:      "string",
				MinLength: int64ptr(1),
			},
			//"lastTransitionTime": Seem to be no way to express "string or null" constraint in schema
			"reason": {
				Type: "string",
			},
			"message": {
				Type: "string",
			},
		},
	}
	resourceStatus := apiext_v1b1.JSONSchemaProps{
		Type:     "object",
		Required: []string{"name"},
		Properties: map[string]apiext_v1b1.JSONSchemaProps{
			"name": {
				Type:      "string",
				MinLength: int64ptr(1),
			},
			"conditions": {
				Type: "array",
				Items: &apiext_v1b1.JSONSchemaPropsOrArray{
					Schema: &condition,
				},
			},
		},
	}
	objectToDelete := apiext_v1b1.JSONSchemaProps{
		Type:     "object",
		Required: []string{"group", "version", "kind", "name"},
		Properties: map[string]apiext_v1b1.JSONSchemaProps{
			"group": {
				Type: "string",
			},
			"version": {
				Type:      "string",
				MinLength: int64ptr(1),
			},
			"kind": {
				Type:      "string",
				MinLength: int64ptr(1),
			},
			"name": {
				Type:      "string",
				MinLength: int64ptr(1),
			},
		},
	}
	pluginStatus := apiext_v1b1.JSONSchemaProps{
		Type:     "object",
		Required: []string{"name", "group", "version", "kind"},
		Properties: map[string]apiext_v1b1.JSONSchemaProps{
			"name": {
				Type:      "string",
				MinLength: int64ptr(1),
			},
			"group": {
				Type: "string",
			},
			"version": {
				Type:      "string",
				MinLength: int64ptr(1),
			},
			"kind": {
				Type:      "string",
				MinLength: int64ptr(1),
			},
			"status": {
				Type: "string",
			},
		},
	}
	bundleStatus := apiext_v1b1.JSONSchemaProps{
		Type: "object",
		Properties: map[string]apiext_v1b1.JSONSchemaProps{
			"conditions": {
				Type: "array",
				Items: &apiext_v1b1.JSONSchemaPropsOrArray{
					Schema: &condition,
				},
			},
			"resourceStatuses": {
				Type: "array",
				Items: &apiext_v1b1.JSONSchemaPropsOrArray{
					Schema: &resourceStatus,
				},
			},
			"objectsToDelete": {
				Type: "array",
				Items: &apiext_v1b1.JSONSchemaPropsOrArray{
					Schema: &objectToDelete,
				},
			},
			"pluginStatuses": {
				Type: "array",
				Items: &apiext_v1b1.JSONSchemaPropsOrArray{
					Schema: &pluginStatus,
				},
			},
		},
	}

	return &apiext_v1b1.CustomResourceDefinition{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "CustomResourceDefinition",
			APIVersion: apiext_v1b1.SchemeGroupVersion.String(),
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: smith_v1.BundleResourceName,
		},
		Spec: apiext_v1b1.CustomResourceDefinitionSpec{
			Group: smith.GroupName,
			Names: apiext_v1b1.CustomResourceDefinitionNames{
				Plural:   smith_v1.BundleResourcePlural,
				Singular: smith_v1.BundleResourceSingular,
				Kind:     smith_v1.BundleResourceKind,
			},
			Scope: apiext_v1b1.NamespaceScoped,
			Validation: &apiext_v1b1.CustomResourceValidation{
				OpenAPIV3Schema: &apiext_v1b1.JSONSchemaProps{
					Required: []string{"spec"},
					Properties: map[string]apiext_v1b1.JSONSchemaProps{
						"spec":   bundleSpec,
						"status": bundleStatus,
					},
				},
			},
			Versions: []apiext_v1b1.CustomResourceDefinitionVersion{
				{
					Name:    smith_v1.BundleResourceVersion,
					Served:  true,
					Storage: true,
				},
			},
		},
	}
}

func int64ptr(val int64) *int64 {
	return &val
}

func EnsureCrdExistsAndIsEstablished(ctx context.Context, logger *zap.Logger, apiExtClient apiExtClientset.Interface, crdLister apiext_lst_v1b1.CustomResourceDefinitionLister, crd *apiext_v1b1.CustomResourceDefinition) error {
	err := EnsureCrdExists(ctx, logger, apiExtClient, crdLister, crd)
	if err != nil {
		return err
	}
	logger.Info("Waiting for CustomResourceDefinition to become established", ctrlLogz.Object(crd))
	return WaitForCrdToBecomeEstablished(ctx, crdLister, crd)
}

func EnsureCrdExists(ctx context.Context, logger *zap.Logger, apiExtClient apiExtClientset.Interface, crdLister apiext_lst_v1b1.CustomResourceDefinitionLister, crd *apiext_v1b1.CustomResourceDefinition) error {
	for {
		obj, err := crdLister.Get(crd.Name)
		notFound := api_errors.IsNotFound(err)
		if err != nil && !notFound {
			return err
		}
		if notFound {
			logger.Info("Creating CustomResourceDefinition", ctrlLogz.Object(crd))
			_, err = apiExtClient.ApiextensionsV1beta1().CustomResourceDefinitions().Create(crd)
			if err == nil {
				logger.Info("CustomResourceDefinition created", ctrlLogz.Object(crd))
				return nil
			}
			if !api_errors.IsAlreadyExists(err) {
				return errors.Wrapf(err, "failed to create %s CustomResourceDefinition", crd.Name)
			}
			logger.Info("CustomResourceDefinition was created concurrently", ctrlLogz.Object(crd))
		} else {
			if IsEqualCrd(crd, obj) {
				return nil
			}
			logger.Info("Updating CustomResourceDefinition", ctrlLogz.Object(crd))
			obj = obj.DeepCopy()
			obj.Spec = crd.Spec
			obj.Annotations = crd.Annotations
			obj.Labels = crd.Labels
			// TODO erasing the status is only necessary because there is no support for generation/observedGeneration at the moment
			obj.Status = apiext_v1b1.CustomResourceDefinitionStatus{}
			_, err = apiExtClient.ApiextensionsV1beta1().CustomResourceDefinitions().Update(obj) // This is a CAS
			if err == nil {
				logger.Info("CustomResourceDefinition updated", ctrlLogz.Object(crd))
				return nil
			}
			if !api_errors.IsConflict(err) {
				return errors.Wrapf(err, "failed to update CustomResourceDefinition %s", crd.Name)
			}
			logger.Info("Conflict updating CustomResourceDefinition, retrying", ctrlLogz.Object(crd))
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

func IsEqualCrd(a, b *apiext_v1b1.CustomResourceDefinition) bool {
	a = a.DeepCopy()
	b = b.DeepCopy()

	apiext_v1b1.SetDefaults_CustomResourceDefinitionSpec(&a.Spec)
	apiext_v1b1.SetDefaults_CustomResourceDefinitionSpec(&b.Spec)

	// Ignoring labels
	as := a.Spec
	bs := b.Spec
	return as.Group == bs.Group &&
		isEqualCrdNames(as.Names, bs.Names) &&
		as.Scope == bs.Scope &&
		isEqualValidation(as.Validation, bs.Validation) &&
		isEqualSubresources(as.Subresources, bs.Subresources) &&
		isEqualVersions(as.Versions, bs.Versions) &&
		isEqualAdditionalPrinterColumns(as.AdditionalPrinterColumns, bs.AdditionalPrinterColumns) &&
		isEqualAnnotations(a.Annotations, b.Annotations)
}

func isEqualCrdNames(a, b apiext_v1b1.CustomResourceDefinitionNames) bool {
	return reflect.DeepEqual(a, b)
}

func isEqualValidation(a, b *apiext_v1b1.CustomResourceValidation) bool {
	return reflect.DeepEqual(a, b)
}

func isEqualSubresources(a, b *apiext_v1b1.CustomResourceSubresources) bool {
	return reflect.DeepEqual(a, b)
}

func isEqualVersions(a, b []apiext_v1b1.CustomResourceDefinitionVersion) bool {
	return reflect.DeepEqual(a, b)
}

func isEqualAdditionalPrinterColumns(a, b []apiext_v1b1.CustomResourceColumnDefinition) bool {
	return reflect.DeepEqual(a, b)
}

func isEqualAnnotations(a, b map[string]string) bool {
	return reflect.DeepEqual(a, b)
}
