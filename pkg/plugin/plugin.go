package plugin

import (
	"github.com/pkg/errors"
	"github.com/xeipuuv/gojsonschema"
)

type Container struct {
	Plugin Plugin
	schema *gojsonschema.Schema
}

type ValidationResult struct {
	Errors []error
}

func NewContainer(newPlugin NewFunc) (Container, error) {
	plugin, err := newPlugin()
	if err != nil {
		return Container{}, errors.Wrap(err, "failed to instantiate plugin")
	}
	description := plugin.Describe()
	var schema *gojsonschema.Schema
	if description.SpecSchema != nil {
		schema, err = gojsonschema.NewSchema(gojsonschema.NewBytesLoader(description.SpecSchema))
		if err != nil {
			return Container{}, errors.Wrapf(err, "can't use plugin %q due to invalid schema", description.Name)
		}
	}

	return Container{
		Plugin: plugin,
		schema: schema,
	}, nil
}

func (pc *Container) ValidateSpec(pluginSpec map[string]interface{}) (ValidationResult, error) {
	if pc.schema == nil {
		return ValidationResult{}, nil
	}

	result, err := pc.schema.Validate(gojsonschema.NewGoLoader(pluginSpec))
	if err != nil {
		return ValidationResult{}, errors.Wrap(err, "error validating plugin spec")
	}

	if !result.Valid() {
		validationErrors := result.Errors()
		errs := make([]error, 0, len(validationErrors))

		for _, validationErr := range validationErrors {
			errs = append(errs, errors.New(validationErr.String()))
		}

		return ValidationResult{errs}, nil
	}

	return ValidationResult{}, nil
}
