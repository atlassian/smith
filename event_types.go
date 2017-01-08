package smith

import (
	"encoding/json"
	"fmt"

	"k8s.io/client-go/pkg/api/unversioned"
	"k8s.io/client-go/pkg/watch"
)

// WatchEventHeader objects are streamed from the api server in response to a watch request.
type WatchEventHeader struct {
	// The type of the watch event; added, modified, deleted, or error.
	Type watch.EventType `json:"type,omitempty" description:"the type of watch event; may be ADDED, MODIFIED, DELETED, or ERROR"`
}

type TemplateWatchEvent struct {
	Type watch.EventType `json:"type"`

	// Object is:
	//  * If Type is Added or Modified: the new state of the object.
	//  * If Type is Deleted: the state of the object immediately before deletion.
	//  * If Type is Error: *api.Status is recommended; other types may make sense
	//    depending on context.
	Object *Template           `json:"object"`
	Status *unversioned.Status `json:"-"`
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

func unmarshalEvent(data []byte, v interface{}, t *watch.EventType, s **unversioned.Status) error {
	var weh WatchEventHeader
	if err := json.Unmarshal(data, &weh); err != nil {
		return err
	}
	*t = weh.Type
	switch weh.Type {
	case watch.Added, watch.Modified, watch.Deleted:
		return json.Unmarshal(data, v)
	case watch.Error:
		var holder struct {
			Object unversioned.Status `json:"object"`
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
