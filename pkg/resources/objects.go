package resources

import (
	"bytes"
	"errors"
	"fmt"

	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/jsonpath"
)

// GetControllerOf returns the controllerRef if controllee has a controller, otherwise returns nil.
func GetControllerOf(controllee meta_v1.Object) *meta_v1.OwnerReference {
	for _, ref := range controllee.GetOwnerReferences() {
		if ref.Controller != nil && *ref.Controller {
			return &ref
		}
	}
	return nil
}

// GetJsonPathString extracts the value from the object using given JsonPath template, in a string format
func GetJsonPathString(obj interface{}, path string) (string, error) {
	j := jsonpath.New("GetJsonPathString")
	// If the key is missing, return an empty string without errors
	j.AllowMissingKeys(true)
	err := j.Parse(path)
	if err != nil {
		return "", fmt.Errorf("JsonPath parse %s error: %v", path, err)
	}
	var buf bytes.Buffer
	err = j.Execute(&buf, obj)
	if err != nil {
		return "", fmt.Errorf("JsonPath execute error: %v", err)
	}
	return buf.String(), nil
}

// GetJsonPathValue extracts the value from the object using given JsonPath template
func GetJsonPathValue(obj interface{}, path string) (interface{}, error) {
	j := jsonpath.New("GetJsonPathValue")
	// If the key is missing, return an empty string without errors
	j.AllowMissingKeys(true)
	err := j.Parse(path)
	if err != nil {
		return nil, fmt.Errorf("JsonPath parse %s error: %v", path, err)
	}
	values, err := j.FindResults(obj)
	if err != nil {
		return nil, fmt.Errorf("JsonPath execute error: %v", err)
	}
	if len(values) == 0 {
		return nil, nil
	}
	if len(values) > 1 {
		return nil, errors.New("single result expected, got many")
	}
	if values[0] == nil {
		return nil, nil
	}
	return values[0][0].Interface(), nil
}
