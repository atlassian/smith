package controller

import (
	"fmt"
	"reflect"
	"regexp"
	"unicode/utf8"

	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/smith/pkg/resources"
	"github.com/pkg/errors"
	"go.uber.org/multierr"
)

const (
	ReferenceModifierBindSecret = "bindsecret"
)

var (
	// ?s allows us to match multiline expressions.
	reference = regexp.MustCompile(`(?s)^(!+)\{(.+)}$`)
)

type SpecProcessor struct {
	variables map[smith_v1.ReferenceName]interface{}
}

// noExampleError occurs when we try to process the spec with examples rather
// than resolving references, but at least one of references doesn't specify an example.
type noExampleError struct {
	referenceName smith_v1.ReferenceName
}

func (e *noExampleError) Error() string {
	return fmt.Sprintf("no example value provided in reference %q", e.referenceName)
}

func NewSpec(resources map[smith_v1.ResourceName]*resourceInfo, references []smith_v1.Reference) (*SpecProcessor, error) {
	variables, err := resolveAllReferences(references, func(reference smith_v1.Reference) (interface{}, error) {
		return resolveReference(resources, reference)
	})

	if err != nil {
		return nil, err
	}

	return &SpecProcessor{
		variables: variables,
	}, nil
}

func NewExamplesSpec(references []smith_v1.Reference) (*SpecProcessor, error) {
	variables, err := resolveAllReferences(references, func(reference smith_v1.Reference) (interface{}, error) {
		if reference.Example == nil {
			return nil, errors.WithStack(&noExampleError{referenceName: reference.Name})
		}
		return reference.Example, nil
	})

	if err != nil {
		return nil, err
	}

	return &SpecProcessor{
		variables: variables,
	}, nil
}

func resolveAllReferences(
	references []smith_v1.Reference,
	resolveReference func(reference smith_v1.Reference) (interface{}, error),
) (map[smith_v1.ReferenceName]interface{}, error) {

	ar := make(map[smith_v1.ReferenceName]interface{}, len(references))
	var finalerr error
	for _, reference := range references {
		// Don't 'resolve' nameless references - they're just being
		// used to cause dependencies.
		if reference.Name == "" {
			continue
		}

		resolvedRef, err := resolveReference(reference)
		if err != nil {
			finalerr = multierr.Append(finalerr, err)
		}
		ar[reference.Name] = resolvedRef
	}

	return ar, finalerr
}

func (sp *SpecProcessor) ProcessObject(obj map[string]interface{}, path ...string) error {
	for key, value := range obj {
		v, err := sp.ProcessValue(value, append(path, key)...)
		if err != nil {
			return err
		}
		obj[key] = v
	}
	return nil
}

func (sp *SpecProcessor) ProcessValue(value interface{}, path ...string) (interface{}, error) {
	switch v := value.(type) {
	case string:
		return sp.ProcessString(v, path...)
	case map[string]interface{}:
		if err := sp.ProcessObject(v, path...); err != nil {
			return nil, err
		}
	default:
		// handle slices and slices of slices and ... inception. err, reflection
		rv := reflect.ValueOf(value)
		if rv.Kind() != reflect.Slice {
			break
		}
		length := rv.Len()
		// this may change underlying slice type and this is on purpose. E.g. it may be a slice of string
		// references, some elements of which need to be turned into structs. That means resulting
		// slice may have mixed types.
		result := make([]interface{}, length)
		for i := 0; i < length; i++ {
			res, err := sp.ProcessValue(rv.Index(i).Interface(), append(path, fmt.Sprintf("[%d]", i))...)
			if err != nil {
				return nil, err
			}
			result[i] = res
		}
		value = result
	}
	return value, nil
}

func (sp *SpecProcessor) ProcessString(value string, path ...string) (interface{}, error) {
	match := reference.FindStringSubmatch(value)
	if match == nil {
		return value, nil
	}

	// TODO escaping.

	reference, allowed := sp.variables[smith_v1.ReferenceName(match[2])]
	if !allowed {
		return nil, errors.Errorf("reference does not exist in resource references block: %s", match[2])
	}

	return reference, nil
}

func resolveReference(resInfos map[smith_v1.ResourceName]*resourceInfo, reference smith_v1.Reference) (interface{}, error) {
	resInfo := resInfos[reference.Resource]
	if resInfo == nil {
		return nil, errors.Errorf("internal dependency resolution error - resource referenced by %q not found in Bundle: %s", reference.Name, reference.Resource)
	}

	var objToTraverse interface{}
	switch reference.Modifier {
	case "":
		objToTraverse = resInfo.actual.Object
	case ReferenceModifierBindSecret:
		if resInfo.serviceBindingSecret == nil {
			return nil, errors.Errorf("%q requested, but %q is not a ServiceBinding", ReferenceModifierBindSecret, reference.Resource)
		}
		objToTraverse = resInfo.serviceBindingSecret
	default:
		return nil, errors.Errorf("reference modifier %q not understood for %q", reference.Modifier, reference.Resource)
	}

	// To avoid overcomplicated format of reference like this: {{res1#{$.a.string}}}
	// And have something like this instead: {{res1#a.string}}
	jsonPath := fmt.Sprintf("{$.%s}", reference.Path)
	fieldValue, err := resources.GetJsonPathValue(objToTraverse, jsonPath, false)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to process reference %q", reference.Name)
	}
	if fieldValue == nil {
		return nil, errors.Errorf("field not found: %q", reference.Path)
	}

	if byteFieldValue, ok := fieldValue.([]byte); ok {
		// Secrets are in bytes. We wildly cast them to a string and hope for the best
		// so we can put them in the JSON in a 'nice' way.
		if !utf8.Valid(byteFieldValue) {
			return nil, errors.Errorf("cannot expand non-UTF8 byte array field %q", reference.Path)
		}
		fieldValue = string(byteFieldValue)
	}

	return fieldValue, nil
}
