package processor

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/atlassian/smith"
	"github.com/atlassian/smith/pkg/resources"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const (
	// . is a valid character in a name, cannot use as a separator.
	// See https://github.com/kubernetes/community/blob/master/contributors/design-proposals/identifiers.md
	ReferenceSep = "/"
)

var (
	// TODO a proper lexer should be use to allow escaping of $ to avoid substitution
	reference             = regexp.MustCompile(`\$\([^()]+\)`)
	nakedReference        = regexp.MustCompile(`^\$\(\([^()]+\)\)$`)
	invalidNakedReference = regexp.MustCompile(`(\$\(\([^()]+\)\).|.\$\(\([^()]+\)\))`)
)

type SpecProcessor struct {
	selfName         smith.ResourceName
	readyResources   map[smith.ResourceName]*unstructured.Unstructured
	allowedResources map[smith.ResourceName]struct{}
}

func NewSpec(selfName smith.ResourceName, readyResources map[smith.ResourceName]*unstructured.Unstructured, allowedResources []smith.ResourceName) *SpecProcessor {
	ar := make(map[smith.ResourceName]struct{}, len(allowedResources))
	for _, allowedResource := range allowedResources {
		ar[allowedResource] = struct{}{}
	}
	return &SpecProcessor{
		selfName:         selfName,
		readyResources:   readyResources,
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
		//TODO handle slices and slices of slices and ... inception. err, reflection
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
			processed, err = sp.processMatch(value[3:len(value)-2], false)
		} else {
			processed = reference.ReplaceAllStringFunc(value, func(match string) string {
				processedValue, e := sp.processMatch(match[2:len(match)-1], true)
				if err == nil {
					err = e
				}
				return fmt.Sprintf("%v", processedValue)
			})
		}
	}
	if err != nil {
		return nil, fmt.Errorf("invalid reference at %q: %v", strings.Join(path, ReferenceSep), err)
	}
	return processed, nil
}

func (sp *SpecProcessor) processMatch(selector string, primitivesOnly bool) (interface{}, error) {
	names := strings.Split(selector, ReferenceSep)
	if len(names) < 2 {
		return nil, fmt.Errorf("cannot include whole object: %s", selector)
	}
	objName := smith.ResourceName(names[0])
	if objName == sp.selfName {
		return nil, fmt.Errorf("self references are not allowed: %s", selector)
	}
	res := sp.readyResources[objName]
	if res == nil {
		return nil, fmt.Errorf("object not found: %s", selector)
	}
	if _, allowed := sp.allowedResources[objName]; !allowed {
		return nil, fmt.Errorf("references can only point at direct dependencies: %s", selector)
	}
	fieldValue := resources.GetNestedField(res.Object, names[1:]...)
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
