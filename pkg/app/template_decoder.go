package app

import (
	"encoding/json"

	"github.com/atlassian/smith"

	"k8s.io/client-go/pkg/runtime"
	"k8s.io/client-go/pkg/watch"
)

type templateDecoder struct {
	decoder *json.Decoder
	close   func() error
}

func (d *templateDecoder) Close() {
	d.close() // #nosec
}

func (d *templateDecoder) Decode() (action watch.EventType, object runtime.Object, err error) {
	var event smith.TemplateWatchEvent
	if err := d.decoder.Decode(&event); err != nil {
		return watch.Error, nil, err
	}
	if event.Type == watch.Error {
		return event.Type, event.Status, nil
	}
	return event.Type, event.Object, nil
}
