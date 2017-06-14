package resources

import (
	"bytes"
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

// GetJsonPathString extracts the value from the object using given JsonPath template
func GetJsonPathString(obj map[string]interface{}, path string) (string, error) {
	j := jsonpath.New("GetJsonPathField")
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
