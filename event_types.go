package smith

import (
	"encoding/json"
	"fmt"
)

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
	var holder struct {
		Object Template `json:"object"`
	}
	if err := unmarshalEvent(data, &holder, &twe.Type, &twe.Status); err != nil {
		return err
	}
	twe.Object = &holder.Object
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
	var holder struct {
		Object TprInstance `json:"object"`
	}
	if err := unmarshalEvent(data, &holder, &twe.Type, &twe.Status); err != nil {
		return err
	}
	twe.Object = &holder.Object
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
	var holder struct {
		Object ThirdPartyResource `json:"object"`
	}
	if err := unmarshalEvent(data, &holder, &twe.Type, &twe.Status); err != nil {
		return err
	}
	twe.Object = &holder.Object
	return nil
}

func (twe *TprWatchEvent) ResourceVersion() string {
	return twe.Object.ResourceVersion
}

func unmarshalEvent(data []byte, v interface{}, t *EventType, s **Status) error {
	var weh WatchEventHeader
	if err := json.Unmarshal(data, &weh); err != nil {
		return err
	}
	*t = weh.Type
	switch weh.Type {
	case Added, Modified, Deleted:
		return json.Unmarshal(data, v)
	case Error:
		var holder struct {
			Object Status `json:"object"`
		}
		if err := json.Unmarshal(data, &holder); err != nil {
			return err
		}
		*s = &holder.Object
	default:
		return fmt.Errorf("unexpected event type: %s", weh.Type)
	}
	return nil
}
