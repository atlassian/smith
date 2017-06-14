package resources

import (
	"bytes"
	"fmt"

	"k8s.io/client-go/util/jsonpath"
)

// GetJsonPathString extracts the value from the object using given JsonPath template
func GetJsonPathString(obj map[string]interface{}, path string) (string, error) {
	j := jsonpath.New("GetJsonPathField")
	// If the key is missing, return an empty string without errors
	j.AllowMissingKeys(true)
	err := j.Parse(path)
	if err != nil {
		return "", fmt.Errorf("JsonPath parse %s error: %v", path, err)
	}
	buf := new(bytes.Buffer)
	err = j.Execute(buf, obj)
	if err != nil {
		return "", fmt.Errorf("JsonPath execute error %v", err)
	}
	out := buf.String()
	return out, nil
}

// The rest of this file was copied from k8s.io/apimachinery/pkg/apis/meta/v1/unstructured/unstructured.go
// Remove once https://github.com/kubernetes/kubernetes/issues/40790 is fixed and available for use.

func GetNestedField(obj map[string]interface{}, fields ...string) interface{} {
	var val interface{} = obj
	for _, field := range fields {
		if _, ok := val.(map[string]interface{}); !ok {
			return nil
		}
		val = val.(map[string]interface{})[field]
	}
	return val
}

func GetNestedString(obj map[string]interface{}, fields ...string) string {
	if str, ok := GetNestedField(obj, fields...).(string); ok {
		return str
	}
	return ""
}
