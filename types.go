package main

type ResourceState string

const (
	NEW         ResourceState = ""
	IN_PROGRESS ResourceState = "InProgress"
	READY       ResourceState = "Ready"
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

type Resource struct {
	TypeMeta `json:",inline"`

	// Standard object metadata
	ObjectMeta `json:"metadata,omitempty"`

	Spec ResourceSpec `json:"spec"`

	// Status is most recently observed status of the Template.
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

type ResourceSpec map[string]interface{}

// GenericEvent represents a single event to a watched resource.
type GenericEvent struct {
	Type EventType `json:"type"`

	// Object is:
	//  * If Type is Added or Modified: the new state of the object.
	//  * If Type is Deleted: the state of the object immediately before deletion.
	//  * If Type is Error: *api.Status is recommended; other types may make sense
	//    depending on context.
	//Object Object
}
