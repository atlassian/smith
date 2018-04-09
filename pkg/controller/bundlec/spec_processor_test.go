package bundlec

import (
	"strconv"
	"testing"

	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	core_v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestSpecProcessor(t *testing.T) {
	t.Parallel()
	sp, err := newSpec(processedResources(), []smith_v1.Reference{
		{
			// Nameless references cause dependencies only.
			Resource: "resX",
		},
		{
			Name:     "res1aslice",
			Resource: "res1",
			Path:     "a.slice[?(@.label==\"label2\")].value",
		},
		{
			Name:     "res1astring",
			Resource: "res1",
			Path:     "a.string",
		},
		{
			Name:     "res1aint",
			Resource: "res1",
			Path:     "a.int",
		},
		{
			Name:     "res1abool",
			Resource: "res1",
			Path:     "a.bool",
		},
		{
			Name:     "res1afloat64",
			Resource: "res1",
			Path:     "a.float64",
		},
		{
			Name:     "res1aobject",
			Resource: "res1",
			Path:     "a.object",
		},
	})
	require.NoError(t, err)
	obj := map[string]interface{}{
		"ref": map[string]interface{}{
			"slice":  "!{res1aslice}",
			"string": "!{res1astring}",

			"int":     "!{res1aint}",
			"bool":    "!{res1abool}",
			"float64": "!{res1afloat64}",
			"object":  "!{res1aobject}",

			"slice1": []string{
				"!{res1aint}",
				"23",
				"!{res1aobject}",
			},
			"slice2": []interface{}{
				map[string]interface{}{
					"x": "!{res1aint}",
				},
				"!{res1aint}",
				23,
				"!{res1aobject}",
			},
			"slice3": [][]interface{}{
				{"!{res1aint}"},
				{23},
				{"!{res1aobject}"},
			},
			"slice4": []interface{}{
				[]interface{}{"!{res1aint}"},
				[]interface{}{23},
				[]interface{}{"!{res1aobject}"},
			},
		},
	}
	expected := map[string]interface{}{
		"ref": map[string]interface{}{
			"slice":  "value2",
			"string": "string1",

			"int":     42,
			"bool":    true,
			"float64": float64(1.1),
			"object": map[string]interface{}{
				"a": 1,
				"b": "str",
			},

			"slice1": []interface{}{
				42,
				"23",
				map[string]interface{}{
					"a": 1,
					"b": "str",
				},
			},
			"slice2": []interface{}{
				map[string]interface{}{
					"x": 42,
				},
				42,
				23,
				map[string]interface{}{
					"a": 1,
					"b": "str",
				},
			},
			"slice3": []interface{}{
				[]interface{}{42},
				[]interface{}{23},
				[]interface{}{map[string]interface{}{
					"a": 1,
					"b": "str",
				}}},
			"slice4": []interface{}{
				[]interface{}{42},
				[]interface{}{23},
				[]interface{}{map[string]interface{}{
					"a": 1,
					"b": "str",
				},
				},
			},
		},
	}

	require.NoError(t, sp.ProcessObject(obj))
	assert.Equal(t, expected, obj)
}

func TestSpecProcessorBindSecret(t *testing.T) {
	t.Parallel()
	sp, err := newSpec(processedResources(), []smith_v1.Reference{
		{
			Name:     "res1aint",
			Resource: "res1",
			Path:     "a.int",
		},
		{
			Name:     "password",
			Resource: "resbinding",
			Path:     "Data.password",
			Modifier: "bindsecret",
		},
	})
	require.NoError(t, err)
	obj := map[string]interface{}{
		"ref": map[string]interface{}{
			"int":    "!{res1aint}",
			"secret": "!{password}",
		},
	}
	expected := map[string]interface{}{
		"ref": map[string]interface{}{
			"int":    42,
			"secret": "secret",
		},
	}

	require.NoError(t, sp.ProcessObject(obj))
	assert.Equal(t, expected, obj)
}

func TestSpecProcessorBindSecretWithJsonField(t *testing.T) {
	// We don't convert the Secret to unstructured so we have base64 decoded 'stuff'.
	// However, kubernetes jsonpath is smart (crazy?) enough to use both the json
	// tags AND the field names in its lookups...
	t.Parallel()
	sp, err := newSpec(processedResources(), []smith_v1.Reference{
		{
			Name:     "res1aint",
			Resource: "res1",
			Path:     "a.int",
		},
		{
			Name:     "password",
			Resource: "resbinding",
			Path:     "data.password",
			Modifier: "bindsecret",
		},
	})
	require.NoError(t, err)
	obj := map[string]interface{}{
		"ref": map[string]interface{}{
			"int":    "!{res1aint}",
			"secret": "!{password}",
		},
	}
	expected := map[string]interface{}{
		"ref": map[string]interface{}{
			"int":    42,
			"secret": "secret",
		},
	}

	require.NoError(t, sp.ProcessObject(obj))
	assert.Equal(t, expected, obj)
}

func TestSpecProcessorExamples(t *testing.T) {
	t.Parallel()
	sp, err := newExamplesSpec([]smith_v1.Reference{
		{
			Name:     "res1aint",
			Resource: "res1",
			Path:     "a.int",
			Example:  25,
		},
		{
			Name:     "password",
			Resource: "resbinding",
			Path:     "data.password",
			Modifier: "bindsecret",
			Example: map[string]interface{}{
				"x": "pass",
			},
		},
	})
	require.NoError(t, err)
	obj := map[string]interface{}{
		"ref": map[string]interface{}{
			"int":    "!{res1aint}",
			"secret": "!{password}",
		},
	}
	expected := map[string]interface{}{
		"ref": map[string]interface{}{
			"int":    25,
			"secret": map[string]interface{}{"x": "pass"},
		},
	}

	require.NoError(t, sp.ProcessObject(obj))
	assert.Equal(t, expected, obj)
}

func TestSpecProcessorErrors(t *testing.T) {
	t.Parallel()
	inputs := []struct {
		obj          map[string]interface{}
		reference    smith_v1.Reference
		err          string
		examplesOnly bool
	}{
		{
			reference: smith_v1.Reference{
				Name:     "x",
				Resource: "res1",
				Path:     "something",
			},
			err: `failed to process reference "x": JsonPath execute error: something is not found`,
		},
		{
			reference: smith_v1.Reference{
				Name:     "x",
				Resource: "res2",
				Path:     "a.string",
			},
			err: `internal dependency resolution error - resource referenced by "x" not found in Bundle: res2`,
		},
		{
			reference: smith_v1.Reference{
				Name:     "x",
				Resource: "res1",
			},
			err: `failed to process reference "x": JsonPath execute error:  is not found`,
		},
		{
			reference: smith_v1.Reference{
				Name:     "x",
				Resource: "res1",
				Path:     "data.password",
				Modifier: "bindsecret",
			},
			err: `"bindsecret" requested, but "res1" is not a ServiceBinding`,
		},
		{
			reference: smith_v1.Reference{
				Name:     "x",
				Resource: "res1",
				Path:     "data.password",
				Modifier: "someotherthing",
			},
			err: `reference modifier "someotherthing" not understood for "res1"`,
		},
		{
			reference: smith_v1.Reference{
				Name:     "password",
				Resource: "resbinding",
				Path:     "data.nonutf8",
				Modifier: "bindsecret",
			},
			err: `cannot expand non-UTF8 byte array field "data.nonutf8"`,
		},
		{
			reference: smith_v1.Reference{
				Name:     "password",
				Resource: "resbinding",
				Path:     "data.nonutf8",
				Modifier: "bindsecret",
			},
			err:          `no example value provided in reference "password"`,
			examplesOnly: true,
		},
	}
	for i, input := range inputs {
		input := input
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()
			var err error
			if input.examplesOnly {
				_, err = newExamplesSpec([]smith_v1.Reference{input.reference})
			} else {
				_, err = newSpec(processedResources(), []smith_v1.Reference{input.reference})
			}
			assert.EqualError(t, err, input.err)
		})
	}
}

func processedResources() map[smith_v1.ResourceName]*resourceInfo {
	return map[smith_v1.ResourceName]*resourceInfo{
		"res1": {
			actual: &unstructured.Unstructured{
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
						"slice": []interface{}{
							map[string]interface{}{
								"label": "label1",
								"value": "value1",
							},
							map[string]interface{}{
								"label": "label2",
								"value": "value2",
							},
						},
					},
				},
			},
			status: resourceStatusReady{},
		},
		"resbinding": {
			actual: &unstructured.Unstructured{},
			status: resourceStatusReady{},
			serviceBindingSecret: &core_v1.Secret{
				Data: map[string][]byte{
					"password": []byte("secret"),
					"nonutf8":  {255, 254, 255},
				},
			},
		},
		"resX": {
			actual: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"a": map[string]interface{}{
						"string": "string1",
					},
				},
			},
			status: resourceStatusReady{},
		},
	}
}
