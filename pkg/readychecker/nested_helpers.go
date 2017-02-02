package readychecker

// The rest of this file was copied from k8s.io/client-go/pkg/apis/meta/v1/unstructured/unstructured.go
// Remove once https://github.com/kubernetes/kubernetes/issues/40790 is fixed and available for use.

/*
Copyright 2015 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

func getNestedField(obj map[string]interface{}, fields ...string) interface{} {
	var val interface{} = obj
	for _, field := range fields {
		if _, ok := val.(map[string]interface{}); !ok {
			return nil
		}
		val = val.(map[string]interface{})[field]
	}
	return val
}

func getNestedString(obj map[string]interface{}, fields ...string) string {
	if str, ok := getNestedField(obj, fields...).(string); ok {
		return str
	}
	return ""
}
