package smith

import (
	"encoding/json"
)

type ResourceState string

const (
	NEW         ResourceState = ""
	IN_PROGRESS ResourceState = "InProgress"
	READY       ResourceState = "Ready"
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

	ThirdPartyResourceGroupVersion = "extensions/v1beta1"
	ThirdPartyResourcePath         = "thirdpartyresources"

	AllNamespaces = ""
	AllResources  = ""
)

type TemplateList struct {
	TypeMeta `json:",inline"`
	// Standard list metadata.
	// More info: http://releases.k8s.io/HEAD/docs/devel/api-conventions.md#metadata
	ListMeta `json:"metadata,omitempty"`

	// Items is a list of templates.
	Items []Template `json:"items"`
}

// Template describes a resources template.
// Specification and status are separate entities as per
// https://releases.k8s.io/release-1.3/docs/devel/api-conventions.md#spec-and-status
type Template struct {
	TypeMeta `json:",inline"`

	// Standard object metadata
	ObjectMeta `json:"metadata,omitempty"`

	// Spec is the specification of the desired behavior of the Template.
	Spec TemplateSpec `json:"spec,omitempty"`

	// Status is most recently observed status of the Template.
	Status TemplateStatus `json:"status,omitempty"`
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
	// Standard object metadata
	//ObjectMeta `json:"metadata,omitempty"`

	// Name of the resource for references.
	Name string

	// Explicit dependencies.
	DependsOn []DependencyRef `json:"dependsOn,omitempty"`

	Spec ResourceSpec `json:"spec"`
}

// ResourceSpec holds a resource specification in a raw JSON form plus decoded metadata.
type ResourceSpec struct {
	TypeMeta

	// Standard object metadata
	ObjectMeta

	// Holds map[string]interface{} for marshaling from/into JSON.
	Resource interface{} `json:",inline"`
}

func (rs *ResourceSpec) UnmarshalJSON(data []byte) error {
	var meta struct {
		TypeMeta   `json:",inline"`
		ObjectMeta `json:"metadata,omitempty"`
	}
	if err := json.Unmarshal(data, &meta); err != nil {
		return err
	}
	rs.TypeMeta = meta.TypeMeta
	rs.ObjectMeta = meta.ObjectMeta
	return json.Unmarshal(data, &rs.Resource)
}

func (rs *ResourceSpec) MarshalJSON() ([]byte, error) {
	r, ok := rs.Resource.(map[string]interface{})
	if !ok {
		data, err := json.Marshal(rs.Resource)
		if err != nil {
			return nil, err
		}
		if err = json.Unmarshal(data, &r); err != nil {
			return nil, err
		}
		rs.Resource = r
	}
	r["metadata"] = &rs.ObjectMeta
	return json.Marshal(rs.Resource)
}
