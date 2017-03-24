package processor

import (
	"testing"

	"github.com/atlassian/smith"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestSpecProcessor(t *testing.T) {
	t.Parallel()
	sp := SpecProcessor{
		selfName: "abc",
		readyResources: map[smith.ResourceName]*unstructured.Unstructured{
			"res1": {
				Object: map[string]interface{}{
					"a": map[string]interface{}{
						"string":  "string1",
						"int":     int(42),
						"bool":    true,
						"float64": float64(1.1),
						"object": map[string]interface{}{
							"a": 1,
							"b": "str",
						},
					},
				},
			},
		},
		allowedResources: map[smith.ResourceName]struct{}{
			"res1": {},
		},
	}
	obj := map[string]interface{}{
		"ref": map[string]interface{}{
			"string":     "str>$(res1/a/string)<str",
			"intStr":     "str>$(res1/a/int)<str",
			"boolStr":    "str>$(res1/a/bool)<str",
			"float64Str": "str>$(res1/a/float64)<str",

			"int":     "$((res1/a/int))",
			"bool":    "$((res1/a/bool))",
			"float64": "$((res1/a/float64))",
			"object":  "$((res1/a/object))",
		},
	}
	expected := map[string]interface{}{
		"ref": map[string]interface{}{
			"string":     "str>string1<str",
			"intStr":     "str>42<str",
			"boolStr":    "str>true<str",
			"float64Str": "str>1.1<str",

			"int":     42,
			"bool":    true,
			"float64": float64(1.1),
			"object": map[string]interface{}{
				"a": 1,
				"b": "str",
			},
		},
	}

	require.NoError(t, sp.ProcessObject(obj))
	assert.Equal(t, expected, obj)
}
