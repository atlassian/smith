package plugin

import (
	"strings"

	"github.com/pkg/errors"
	"github.com/xeipuuv/gojsonschema"
)

// TODO bad name for this
type PluginContainer struct {
	Plugin Plugin
	schema *gojsonschema.Schema
}

func NewPluginContainer(newPlugin NewFunc) (PluginContainer, error) {
	var err error
	pc := PluginContainer{}
	pc.Plugin, err = newPlugin()
	if err != nil {
		return pc, errors.Wrapf(err, "failed to instantiate plugin %T", pc.Plugin)
	}
	description := pc.Plugin.Describe()
	if description.SpecSchema != nil {
		pc.schema, err = gojsonschema.NewSchema(gojsonschema.NewBytesLoader(description.SpecSchema))
		if err != nil {
			return pc, errors.Wrapf(err, "can't use plugin %q due to invalid schema", description.Name)
		}
	}

	return pc, nil
}

func (pc *PluginContainer) ValidateSpec(pluginSpec map[string]interface{}) error {
	if pc.schema == nil {
		return nil
	}

	validationResult, err := pc.schema.Validate(gojsonschema.NewGoLoader(pluginSpec))
	if err != nil {
		return errors.Wrapf(err, "error validating plugin spec")
	}

	if !validationResult.Valid() {
		validationErrors := validationResult.Errors()
		msgs := make([]string, 0, len(validationErrors))

		for _, validationErr := range validationErrors {
			msgs = append(msgs, validationErr.String())
		}

		return errors.Errorf("spec failed validation against schema: %s",
			strings.Join(msgs, ", "))
	}

	return nil
}
