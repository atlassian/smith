package smith

import (
	"encoding/json"

	"k8s.io/client-go/pkg/api/meta"
	apiv1 "k8s.io/client-go/pkg/api/v1"
	metav1 "k8s.io/client-go/pkg/apis/meta/v1"
	"k8s.io/client-go/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/pkg/runtime/schema"
)

type ResourceState string

const (
	NEW            ResourceState = ""
	IN_PROGRESS    ResourceState = "InProgress"
	READY          ResourceState = "Ready"
	ERROR          ResourceState = "Error"
	TERMINAL_ERROR ResourceState = "TerminalError"
)

const (
	SmithDomain        = "smith.atlassian.com"
	SmithResourceGroup = SmithDomain

	TemplateResourcePath         = "templates"
	TemplateResourceName         = "template." + SmithDomain
	TemplateResourceVersion      = "v1"
	TemplateResourceKind         = "Template"
	TemplateResourceGroupVersion = SmithResourceGroup + "/" + TemplateResourceVersion

	TemplateNameLabel = TemplateResourceName + "/TemplateName"

	// See docs/design/managing-resources.md
	TprFieldPathAnnotation  = SmithDomain + "/TprReadyWhenFieldPath"
	TprFieldValueAnnotation = SmithDomain + "/TprReadyWhenFieldValue"
)

type TemplateList struct {
	metav1.TypeMeta `json:",inline"`
	// Standard list metadata.
	Metadata metav1.ListMeta `json:"metadata,omitempty"`

	// Items is a list of templates.
	Items []Template `json:"items"`
}

// GetObjectKind is required to satisfy Object interface.
func (tl *TemplateList) GetObjectKind() schema.ObjectKind {
	return &tl.TypeMeta
}

// GetListMeta is required to satisfy ListMetaAccessor interface.
func (tl *TemplateList) GetListMeta() metav1.List {
	return &tl.Metadata
}

// Template describes a resources template.
type Template struct {
	metav1.TypeMeta `json:",inline"`

	// Standard object metadata
	Metadata apiv1.ObjectMeta `json:"metadata,omitempty"`

	// Spec is the specification of the desired behavior of the Template.
	Spec TemplateSpec `json:"spec,omitempty"`

	// Status is most recently observed status of the Template.
	Status TemplateStatus `json:"status,omitempty"`
}

// Required to satisfy Object interface
func (t *Template) GetObjectKind() schema.ObjectKind {
	return &t.TypeMeta
}

// Required to satisfy ObjectMetaAccessor interface
func (t *Template) GetObjectMeta() meta.Object {
	return &t.Metadata
}

type TemplateSpec struct {
	Resources []Resource `json:"resources"`
}

type TemplateStatus struct {
	State ResourceState `json:"state,omitempty"`
}

// DependencyRef is a reference to another Resource in the same template.
type DependencyRef string

type Resource struct {
	// Name of the resource for references.
	Name string

	// Explicit dependencies.
	DependsOn []DependencyRef `json:"dependsOn,omitempty"`

	Spec unstructured.Unstructured `json:"spec"`
}

// The code below is used only to work around a known problem with third-party
// resources and ugorji. If/when these issues are resolved, the code below
// should no longer be required.

type templateListCopy TemplateList
type templateCopy Template

func (e *Template) UnmarshalJSON(data []byte) error {
	tmp := templateCopy{}
	err := json.Unmarshal(data, &tmp)
	if err != nil {
		return err
	}
	*e = Template(tmp)
	return nil
}

func (el *TemplateList) UnmarshalJSON(data []byte) error {
	tmp := templateListCopy{}
	err := json.Unmarshal(data, &tmp)
	if err != nil {
		return err
	}
	*el = TemplateList(tmp)
	return nil
}
