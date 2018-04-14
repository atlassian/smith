package resources

import (
	"bytes"

	"github.com/pkg/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/jsonpath"
)

// GetJsonPathString extracts the value from the object using given JsonPath template, in a string format
func GetJsonPathString(obj interface{}, path string) (string, error) {
	j := jsonpath.New("GetJsonPathString")
	// If the key is missing, return an empty string without errors
	j.AllowMissingKeys(true)
	err := j.Parse(path)
	if err != nil {
		return "", errors.Wrapf(err, "JsonPath parse %s error", path)
	}
	var buf bytes.Buffer
	err = j.Execute(&buf, obj)
	if err != nil {
		return "", errors.Wrap(err, "JsonPath execute error")
	}
	return buf.String(), nil
}

// GetJsonPathValue extracts the value from the object using given JsonPath template
func GetJsonPathValue(obj interface{}, path string, allowMissingKeys bool) (interface{}, error) {
	j := jsonpath.New("GetJsonPathValue")
	// If the key is missing, return an empty string without errors
	j.AllowMissingKeys(allowMissingKeys)
	err := j.Parse(path)
	if err != nil {
		return "", errors.Wrapf(err, "JsonPath parse %s error", path)
	}
	values, err := j.FindResults(obj)
	if err != nil {
		return "", errors.Wrap(err, "JsonPath execute error")
	}
	if len(values) == 0 {
		return nil, nil
	}
	if len(values) > 1 {
		return nil, errors.Errorf("single result expected, got %d", len(values))
	}
	if values[0] == nil || len(values[0]) == 0 || values[0][0].IsNil() {
		return nil, nil
	}
	return values[0][0].Interface(), nil
}

func HasFinalizer(accessor meta_v1.Object, finalizer string) bool {
	finalizers := accessor.GetFinalizers()
	for _, f := range finalizers {
		if f == finalizer {
			return true
		}
	}
	return false
}
