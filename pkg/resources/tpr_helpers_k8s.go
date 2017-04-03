package resources

// The rest of this file was copied from k8s.io/kubernetes/pkg/registry/extensions/thirdpartyresourcedata/util.go
// and slightly modified.
// Remove once https://github.com/kubernetes/client-go/issues/165 is fixed and available for use.

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

import (
	"bytes"
	"fmt"
	"strings"
	"unicode"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

func convertToCamelCase(input string) string {
	var buf bytes.Buffer
	toUpper := true
	for _, char := range input {
		if toUpper {
			buf.WriteRune(unicode.ToUpper(char))
			toUpper = false
		} else if char == '-' {
			toUpper = true
		} else {
			buf.WriteRune(char)
		}
	}
	return buf.String()
}

func ExtractApiGroupAndKind(tprName string) (schema.GroupKind, error) {
	parts := strings.SplitN(tprName, ".", 3)
	if len(parts) < 3 {
		return schema.GroupKind{}, fmt.Errorf("unexpectedly short resource name: %s, expected at least <kind>.<domain>.<tld>", tprName)
	}
	return schema.GroupKind{
		Kind:  convertToCamelCase(parts[0]),
		Group: strings.Join(parts[1:], "."),
	}, nil
}
