package controller

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"unicode/utf8"

	"encoding/json"

	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/smith/pkg/resources"
	"github.com/pkg/errors"
)

const (
	// Separator between reference to a resource by name and JsonPath within a resource
	ReferenceSeparator = "#"
	// Separator between dependency and type of output (i.e. resolve a dependency
	// to some produced object)
	ResourceOutputSeparator      = ":"
	ResourceOutputNameBindSecret = "bindsecret"
)

var (
	reference = regexp.MustCompile(`^\{\{.+}}$`)
)

type SpecProcessor struct {
	selfName         smith_v1.ResourceName
	resources        map[smith_v1.ResourceName]*resourceInfo
	allowedResources map[smith_v1.ResourceName]struct{}
	defaultsOnly     bool
}

// noDefaultValueError occurs when we try to process the spec according to defaults,
// but at least one of references doesn't specify a default.
type noDefaultValueError struct {
	selector string
}

func (e *noDefaultValueError) Error() string {
	return fmt.Sprintf("no default value provided in selector %q", e.selector)
}

func NewSpec(selfName smith_v1.ResourceName, resources map[smith_v1.ResourceName]*resourceInfo, allowedResources []smith_v1.ResourceName) *SpecProcessor {
	return &SpecProcessor{
		selfName:         selfName,
		resources:        resources,
		allowedResources: convertResourceNamesToMap(allowedResources),
		defaultsOnly:     false,
	}
}

func NewDefaultsSpec(selfName smith_v1.ResourceName, allowedResources []smith_v1.ResourceName) *SpecProcessor {
	return &SpecProcessor{
		selfName:         selfName,
		allowedResources: convertResourceNamesToMap(allowedResources),
		defaultsOnly:     true,
	}
}

func convertResourceNamesToMap(resources []smith_v1.ResourceName) map[smith_v1.ResourceName]struct{} {
	ar := make(map[smith_v1.ResourceName]struct{}, len(resources))
	for _, allowedResource := range resources {
		ar[allowedResource] = struct{}{}
	}
	return ar
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
	if !reference.MatchString(value) {
		return value, nil
	}

	result, err := sp.processMatch(value[2 : len(value)-2])
	if err != nil {
		return nil, errors.Wrapf(err, "invalid reference at \"%s\"", strings.Join(path, "."))
	}
	return result, nil
}

func (sp *SpecProcessor) processMatch(selector string) (interface{}, error) {
	parts := strings.SplitN(selector, ReferenceSeparator, 3)
	if len(parts) < 2 {
		return nil, errors.Errorf("cannot include whole object: %s", selector)
	}
	objNameParts := strings.SplitN(parts[0], ResourceOutputSeparator, 2)
	objName := smith_v1.ResourceName(objNameParts[0])
	resourceOutputName := ""
	if len(objNameParts) == 2 {
		resourceOutputName = objNameParts[1]
	}
	if objName == sp.selfName {
		return nil, errors.Errorf("self references are not allowed: %s", selector)
	}

	if _, allowed := sp.allowedResources[objName]; !allowed {
		return nil, errors.Errorf("references can only point at direct dependencies: %s", selector)
	}

	if sp.defaultsOnly {
		if len(parts) < 3 {
			return nil, errors.WithStack(&noDefaultValueError{selector})
		}
		var defaultValue interface{}
		if err := json.Unmarshal([]byte(parts[2]), &defaultValue); err != nil {
			return nil, errors.Wrapf(err, "failed to parse default %q", parts[2])
		}
		return defaultValue, nil
	}

	resInfo := sp.resources[objName]
	if resInfo == nil {
		return nil, errors.Errorf("object not found: %s", selector)
	}

	var objToTraverse interface{}
	switch resourceOutputName {
	case "":
		objToTraverse = resInfo.actual.Object
	case ResourceOutputNameBindSecret:
		if resInfo.serviceBindingSecret == nil {
			return nil, errors.Errorf("%q requested, but %q is not a ServiceBinding", resourceOutputName, objName)
		}
		objToTraverse = resInfo.serviceBindingSecret
	default:
		return nil, errors.Errorf("resource output name %q not understood for %q", resourceOutputName, objName)
	}

	// To avoid overcomplicated format of reference like this: {{{res1#{$.a.string}}}}
	// And have something like this instead: {{{res1#a.string}}}
	jsonPath := fmt.Sprintf("{$.%s}", parts[1])
	fieldValue, err := resources.GetJsonPathValue(objToTraverse, jsonPath, false)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to process JsonPath reference %s", selector)
	}
	if fieldValue == nil {
		return nil, errors.Errorf("field not found: %s", selector)
	}

	if byteFieldValue, ok := fieldValue.([]byte); ok {
		// Secrets are in bytes. We wildly cast them to a string and hope for the best
		// so we can put them in the JSON in a 'nice' way.
		if !utf8.Valid(byteFieldValue) {
			return nil, errors.Errorf("cannot expand non-UTF8 byte array field %q", selector)
		}
		fieldValue = string(byteFieldValue)
	}

	return fieldValue, nil
}
