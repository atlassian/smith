package smith

import (
	"encoding/json"
	"fmt"
)

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

// WatchEventHeader objects are streamed from the api server in response to a watch request.
type WatchEventHeader struct {
	// The type of the watch event; added, modified, deleted, or error.
	Type EventType `json:"type,omitempty" description:"the type of watch event; may be ADDED, MODIFIED, DELETED, or ERROR"`
}

type TemplateWatchEvent struct {
	Type EventType `json:"type"`

	// Object is:
	//  * If Type is Added or Modified: the new state of the object.
	//  * If Type is Deleted: the state of the object immediately before deletion.
	//  * If Type is Error: *api.Status is recommended; other types may make sense
	//    depending on context.
	Object *Template `json:"object"`
	Status *Status   `json:"-"`
}

func (twe *TemplateWatchEvent) UnmarshalJSON(data []byte) error {
	var weh WatchEventHeader
	if err := json.Unmarshal(data, &weh); err != nil {
		return err
	}
	twe.Type = weh.Type
	switch weh.Type {
	case Added, Modified, Deleted:
		var holder struct {
			Object Template `json:"object"`
		}
		if err := json.Unmarshal(data, &holder); err != nil {
			return err
		}
		twe.Object = &holder.Object
	case Error:
		var holder struct {
			Object Status `json:"object"`
		}
		if err := json.Unmarshal(data, &holder); err != nil {
			return err
		}
		twe.Status = &holder.Object
	default:
		return fmt.Errorf("unexpected event type: %s", weh.Type)
	}
	return nil
}

func (twe *TemplateWatchEvent) ResourceVersion() string {
	return twe.Object.ResourceVersion
}

// TprInstance describes some Third Party Resource instance.
// It contains only metadata about the object.
type TprInstance struct {
	TypeMeta `json:",inline"`

	// Standard object metadata
	ObjectMeta `json:"metadata,omitempty"`
}

// TprInstanceWatchEvent describes a watch event for some Third Party Resource instance.
type TprInstanceWatchEvent struct {
	Type EventType `json:"type"`

	// Object is:
	//  * If Type is Added or Modified: the new state of the object.
	//  * If Type is Deleted: the state of the object immediately before deletion.
	//  * If Type is Error: *api.Status is recommended; other types may make sense
	//    depending on context.
	Object *TprInstance `json:"object"`
	Status *Status      `json:"-"`
}

func (twe *TprInstanceWatchEvent) UnmarshalJSON(data []byte) error {
	var weh WatchEventHeader
	if err := json.Unmarshal(data, &weh); err != nil {
		return err
	}
	twe.Type = weh.Type
	switch weh.Type {
	case Added, Modified, Deleted:
		var holder struct {
			Object TprInstance `json:"object"`
		}
		if err := json.Unmarshal(data, &holder); err != nil {
			return err
		}
		twe.Object = &holder.Object
	case Error:
		var holder struct {
			Object Status `json:"object"`
		}
		if err := json.Unmarshal(data, &holder); err != nil {
			return err
		}
		twe.Status = &holder.Object
	default:
		return fmt.Errorf("unexpected event type: %s", weh.Type)
	}
	return nil
}

func (twe *TprInstanceWatchEvent) ResourceVersion() string {
	return twe.Object.ResourceVersion
}

// TprWatchEvent describes a watch event for Third Party Resource.
type TprWatchEvent struct {
	Type EventType `json:"type"`

	// Object is:
	//  * If Type is Added or Modified: the new state of the object.
	//  * If Type is Deleted: the state of the object immediately before deletion.
	//  * If Type is Error: *api.Status is recommended; other types may make sense
	//    depending on context.
	Object *ThirdPartyResource `json:"object"`
	Status *Status             `json:"-"`
}

func (twe *TprWatchEvent) UnmarshalJSON(data []byte) error {
	var weh WatchEventHeader
	if err := json.Unmarshal(data, &weh); err != nil {
		return err
	}
	twe.Type = weh.Type
	switch weh.Type {
	case Added, Modified, Deleted:
		var holder struct {
			Object ThirdPartyResource `json:"object"`
		}
		if err := json.Unmarshal(data, &holder); err != nil {
			return err
		}
		twe.Object = &holder.Object
	case Error:
		var holder struct {
			Object Status `json:"object"`
		}
		if err := json.Unmarshal(data, &holder); err != nil {
			return err
		}
		twe.Status = &holder.Object
	default:
		return fmt.Errorf("unexpected event type: %s", weh.Type)
	}
	return nil
}

func (twe *TprWatchEvent) ResourceVersion() string {
	return twe.Object.ResourceVersion
}
