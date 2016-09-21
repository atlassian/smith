package smith

import "encoding/json"

type ResourceState string

const (
	NEW         ResourceState = ""
	IN_PROGRESS ResourceState = "InProgress"
	READY       ResourceState = "Ready"
)

const (
	SmithDomain        = "smith.ash2k.com"
	SmithResourceGroup = SmithDomain

	TemplateResourcePath         = "templates"
	TemplateResourceName         = "template." + SmithDomain
	TemplateResourceVersion      = "v1"
	TemplateResourceGroupVersion = SmithResourceGroup + "/" + TemplateResourceVersion

	TemplateNameLabel = TemplateResourceName + "/templateName"

	ThirdPartyResourceGroupVersion = "extensions/v1beta1"

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
	Spec TemplateSpec `json:"spec"`

	// Status is most recently observed status of the Template.
	Status TemplateStatus `json:"status,omitempty"`
}

type TemplateSpec struct {
	Resources []Resource `json:"resources"`
}

type TemplateStatus struct {
	ResourceStatus `json:",inline"`
}

// DependencyRef is a reference to another Resource in the same template.
type DependencyRef string

type Resource struct {
	// Standard object metadata
	ObjectMeta `json:"metadata,omitempty"`

	// Explicit dependencies
	DependsOn []DependencyRef `json:"dependsOn,omitempty"`

	Spec ResourceSpec `json:"spec"`

	// Status is most recently observed status of the Resource.
	Status ResourceStatus `json:"status,omitempty"`
}

type ResourceStatus struct {
	State   ResourceState    `json:"state,omitempty"`
	Outputs []ResourceOutput `json:"outputs,omitempty"`
}

type ResourceOutput struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
	// TODO support object outputs e.g. secrets
	StringValue string `json:"strValue,omitempty"`
	IntValue    int64  `json:"intValue,omitempty"`
}

// ResourceSpec holds a resource specification in a raw form plus decoded metadata.
type ResourceSpec struct {
	TypeMeta

	// Standard object metadata
	ObjectMeta

	RawResource json.RawMessage
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
	return rs.RawResource.UnmarshalJSON(data)
}
