package controller

import (
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strings"

	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/smith/pkg/resources"
)

const (
	// Separator between reference to a resource by name and JsonPath within a resource
	ReferenceSeparator = "#"
)

var (
	reference             = regexp.MustCompile(`\{\{.+}}`)
	nakedReference        = regexp.MustCompile(`^\{\{\{.+}}}$`)
	invalidNakedReference = regexp.MustCompile(`(\{\{\{.+}}}.|.\{\{\{.+}}})`)
)

type SpecProcessor struct {
	selfName         smith_v1.ResourceName
	resources        map[smith_v1.ResourceName]*resourceInfo
	allowedResources map[smith_v1.ResourceName]struct{}
}

func NewSpec(selfName smith_v1.ResourceName, resources map[smith_v1.ResourceName]*resourceInfo, allowedResources []smith_v1.ResourceName) *SpecProcessor {
	ar := make(map[smith_v1.ResourceName]struct{}, len(allowedResources))
	for _, allowedResource := range allowedResources {
		ar[allowedResource] = struct{}{}
	}
	return &SpecProcessor{
		selfName:         selfName,
		resources:        resources,
		allowedResources: ar,
	}
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
	var err error
	var processed interface{}
	if invalidNakedReference.MatchString(value) {
		err = errors.New("naked reference in the middle of a string")
	} else {
		if nakedReference.MatchString(value) {
			processed, err = sp.processMatch(value[3:len(value)-3], false)
		} else {
			processed = reference.ReplaceAllStringFunc(value, func(match string) string {
				processedValue, e := sp.processMatch(match[2:len(match)-2], true)
				if err == nil {
					err = e
				}
				return fmt.Sprintf("%v", processedValue)
			})
		}
	}
	if err != nil {
		return nil, fmt.Errorf("invalid reference at %q: %v", strings.Join(path, ReferenceSeparator), err)
	}
	return processed, nil
}

func (sp *SpecProcessor) processMatch(selector string, primitivesOnly bool) (interface{}, error) {
	parts := strings.SplitN(selector, ReferenceSeparator, 2)
	if len(parts) < 2 {
		return nil, fmt.Errorf("cannot include whole object: %s", selector)
	}
	objName := smith_v1.ResourceName(parts[0])
	if objName == sp.selfName {
		return nil, fmt.Errorf("self references are not allowed: %s", selector)
	}
	resInfo := sp.resources[objName]
	if resInfo == nil {
		return nil, fmt.Errorf("object not found: %s", selector)
	}
	if _, allowed := sp.allowedResources[objName]; !allowed {
		return nil, fmt.Errorf("references can only point at direct dependencies: %s", selector)
	}
	// To avoid overcomplicated format of reference like this: {{{res1#{$.a.string}}}}
	// And have something like this instead: {{{res1#a.string}}}
	jsonPath := fmt.Sprintf("{$.%s}", parts[1])
	fieldValue, err := resources.GetJsonPathValue(resInfo.actual.Object, jsonPath, false)
	if err != nil {
		return nil, fmt.Errorf("failed to process JsonPath reference %s: %v", selector, err)
	}
	if fieldValue == nil {
		return nil, fmt.Errorf("field not found: %s", selector)
	}
	if primitivesOnly {
		switch fieldValue.(type) {
		case int, uint:
		case string, bool:
		case float32, float64:
		case uint8, uint16, uint32, uint64:
		case int8, int16, int32, int64:
		case complex64, complex128:
		default:
			return nil, fmt.Errorf("cannot expand non-primitive field %s of type %T", selector, fieldValue)
		}
	} else {
		if _, ok := fieldValue.(string); ok {
			return nil, fmt.Errorf("cannot expand field %s of type string as naked reference", selector)
		}
	}
	return fieldValue, nil
}
