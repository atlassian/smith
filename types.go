package smith

import (
	"encoding/json"
	"log"

	"k8s.io/client-go/pkg/api/meta"
	"k8s.io/client-go/pkg/api/unversioned"
	apiv1 "k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/pkg/runtime"
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

	TemplateNameLabel = TemplateResourceName + "/templateName"
)

type TemplateList struct {
	unversioned.TypeMeta `json:",inline"`
	// Standard list metadata.
	Metadata unversioned.ListMeta `json:"metadata,omitempty"`

	// Items is a list of templates.
	Items []Template `json:"items"`
}

// GetObjectKind is required to satisfy Object interface.
func (tl *TemplateList) GetObjectKind() unversioned.ObjectKind {
	return &tl.TypeMeta
}

// GetListMeta is required to satisfy ListMetaAccessor interface.
func (tl *TemplateList) GetListMeta() meta.List {
	return &tl.Metadata
}

// Template describes a resources template.
type Template struct {
	unversioned.TypeMeta `json:",inline"`

	// Standard object metadata
	Metadata apiv1.ObjectMeta `json:"metadata,omitempty"`

	// Spec is the specification of the desired behavior of the Template.
	Spec TemplateSpec `json:"spec,omitempty"`

	// Status is most recently observed status of the Template.
	Status TemplateStatus `json:"status,omitempty"`
}

// Required to satisfy Object interface
func (t *Template) GetObjectKind() unversioned.ObjectKind {
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

	Spec runtime.Unstructured `json:"spec"`
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
	tmp2 := Template(tmp)
	*e = tmp2
	log.Printf("%s", data)
	return nil
}

func (el *TemplateList) UnmarshalJSON(data []byte) error {
	tmp := templateListCopy{}
	err := json.Unmarshal(data, &tmp)
	if err != nil {
		return err
	}
	tmp2 := TemplateList(tmp)
	*el = tmp2
	return nil
}
