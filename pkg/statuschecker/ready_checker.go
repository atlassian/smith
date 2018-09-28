package statuschecker

import (
	"fmt"

	"github.com/atlassian/smith"
	"github.com/atlassian/smith/pkg/resources"
	"github.com/pkg/errors"
	apiext_v1b1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type ObjectStatusType string

const (
	ObjectStatusTypeReady      ObjectStatusType = "Ready"
	ObjectStatusTypeUnknown    ObjectStatusType = "Unknown"
	ObjectStatusTypeInProgress ObjectStatusType = "InProgress"
	ObjectStatusTypeError      ObjectStatusType = "Error"
)

type ObjectStatusResult interface {
	StatusType() ObjectStatusType
}

type ObjectStatusReady struct{}
type ObjectStatusUnknown struct {
	Details string
}
type ObjectStatusInProgress struct {
	Message string
}
type ObjectStatusError struct {
	UserError      bool
	RetriableError bool
	Error          error
}

func (o ObjectStatusReady) StatusType() ObjectStatusType {
	return ObjectStatusTypeReady
}
func (o ObjectStatusUnknown) StatusType() ObjectStatusType {
	return ObjectStatusTypeUnknown
}
func (o ObjectStatusInProgress) StatusType() ObjectStatusType {
	return ObjectStatusTypeInProgress
}
func (o ObjectStatusError) StatusType() ObjectStatusType {
	return ObjectStatusTypeError
}

// ObjectStatusChecker checks object's status.
// Function is responsible for handling different versions of objects by itself.
type ObjectStatusChecker func(runtime.Object) (ObjectStatusResult, error)

// CRDStore gets a CRD definition for a Group and Kind of the resource (CRD instance).
// Returns nil if CRD definition was not found.
type CRDStore interface {
	Get(resource schema.GroupKind) (*apiext_v1b1.CustomResourceDefinition, error)
}

type Interface interface {
	CheckStatus(*unstructured.Unstructured) (r ObjectStatusResult, e error)
}

type Checker struct {
	Store      CRDStore
	KnownTypes map[schema.GroupKind]ObjectStatusChecker
}

func New(store CRDStore, kts ...map[schema.GroupKind]ObjectStatusChecker) (*Checker, error) {
	kt := make(map[schema.GroupKind]ObjectStatusChecker)
	for _, knownTypes := range kts {
		for knownGK, f := range knownTypes {
			if kt[knownGK] != nil {
				return nil, errors.Errorf("GroupKind specified more than once: %s", knownGK)
			}
			kt[knownGK] = f
		}
	}
	return &Checker{
		Store:      store,
		KnownTypes: kt,
	}, nil
}

func (c *Checker) CheckStatus(obj *unstructured.Unstructured) (ObjectStatusResult, error) {
	gvk := obj.GroupVersionKind()
	gk := gvk.GroupKind()

	if gk.Kind == "" || gvk.Version == "" { // Group can be empty e.g. built-in objects like ConfigMap
		return ObjectStatusUnknown{}, errors.Errorf("object has empty kind/version: %s", gvk)
	}

	// 1. Check if it is a known built-in resource
	if isObjectReady, ok := c.KnownTypes[gk]; ok {
		return isObjectReady(obj)
	}

	// 2. Check if it is a CRD with path/value annotation
	crd, err := c.crdWithPathValueAnnotation(gk)
	if err != nil {
		return nil, err
	}
	if crd != nil {
		return c.checkPathValue(crd, obj)
	}

	// 3. Check if it is a CRD with Kind/GroupVersion annotation
	crd, err = c.crdWithKindGroupVersionAnnotation(gk)
	if err != nil {
		return nil, err
	}
	if crd != nil {
		return c.checkForInstance(crd, obj)
	}

	return ObjectStatusInProgress{}, nil
}

func (c *Checker) crdWithKindGroupVersionAnnotation(gk schema.GroupKind) (*apiext_v1b1.CustomResourceDefinition, error) {
	// Not yet implemented
	return nil, nil
}

func (c *Checker) checkForInstance(crd *apiext_v1b1.CustomResourceDefinition, obj *unstructured.Unstructured) (ObjectStatusResult, error) {
	// Not yet implemented
	return ObjectStatusInProgress{}, nil
}

func (c *Checker) crdWithPathValueAnnotation(gk schema.GroupKind) (*apiext_v1b1.CustomResourceDefinition, error) {
	crd, err := c.Store.Get(gk)
	if err != nil {
		return nil, err
	}
	if crd == nil {
		return nil, nil
	}
	path := crd.Annotations[smith.CrFieldPathAnnotation]
	value := crd.Annotations[smith.CrFieldValueAnnotation]
	if len(path) == 0 || len(value) == 0 {
		return nil, nil
	}
	return crd, nil
}

func (c *Checker) checkPathValue(crd *apiext_v1b1.CustomResourceDefinition, obj *unstructured.Unstructured) (ObjectStatusResult, error) {
	path := crd.Annotations[smith.CrFieldPathAnnotation]
	value := crd.Annotations[smith.CrFieldValueAnnotation]
	actualValue, err := resources.GetJSONPathString(obj.Object, path)
	if err != nil {
		// invalid jsonpath annotations on CRD
		return ObjectStatusError{
			UserError: true,
			Error:     err,
		}, nil
	}
	if actualValue != value {
		return ObjectStatusInProgress{
			Message: fmt.Sprintf("Path %q for object still missing value %q", path, value),
		}, nil
	}
	return ObjectStatusReady{}, nil
}
