package crd

import (
	"github.com/atlassian/smith/pkg/apis/smith"
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	apiext_v1b1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

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
			"observedGeneration": {
				Type:   "integer",
				Format: "int64",
			},
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
			Subresources: &apiext_v1b1.CustomResourceSubresources{
				Status: &apiext_v1b1.CustomResourceSubresourceStatus{},
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
