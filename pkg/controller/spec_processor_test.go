package controller

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
	sp := SpecProcessor{
		selfName:  "abc",
		resources: processedResources(),
		allowedResources: map[smith_v1.ResourceName]struct{}{
			"res1": {},
		},
	}
	obj := map[string]interface{}{
		"ref": map[string]interface{}{
			"slice":  "{{res1#a.slice[?(@.label==\"label2\")].value}}",
			"string": "{{res1#a.string}}",

			"int":     "{{res1#a.int}}",
			"bool":    "{{res1#a.bool}}",
			"float64": "{{res1#a.float64}}",
			"object":  "{{res1#a.object}}",

			"slice1": []string{
				"{{res1#a.int}}",
				"23",
				"{{res1#a.object}}",
			},
			"slice2": []interface{}{
				map[string]interface{}{
					"x": "{{res1#a.int}}",
				},
				"{{res1#a.int}}",
				23,
				"{{res1#a.object}}",
			},
			"slice3": [][]interface{}{
				{"{{res1#a.int}}"},
				{23},
				{"{{res1#a.object}}"},
			},
			"slice4": []interface{}{
				[]interface{}{"{{res1#a.int}}"},
				[]interface{}{23},
				[]interface{}{"{{res1#a.object}}"},
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
	sp := SpecProcessor{
		selfName:  "abc",
		resources: processedResources(),
		allowedResources: map[smith_v1.ResourceName]struct{}{
			"res1":       {},
			"resbinding": {},
		},
	}
	obj := map[string]interface{}{
		"ref": map[string]interface{}{
			"int":    "{{res1#a.int}}",
			"secret": "{{resbinding:bindsecret#Data.password}}",
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

func TestSpecProcessorDefaults(t *testing.T) {
	t.Parallel()
	sp := NewDefaultsSpec("abc", []smith_v1.ResourceName{"res1", "resbinding"})
	obj := map[string]interface{}{
		"ref": map[string]interface{}{
			"int":    "{{res1#a.int#25}}",
			"secret": "{{resbinding:bindsecret#Data.password#{\"x\":\"pass\"}}}",
		},
	}
	expected := map[string]interface{}{
		"ref": map[string]interface{}{
			"int":    float64(25), // because JSON only believes in floats...
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
		err          string
		defaultsOnly bool
	}{
		{
			obj: map[string]interface{}{
				"invalid": "{{res1#something}}",
			},
			err: `invalid reference at "invalid": failed to process JsonPath reference res1#something: JsonPath execute error: something is not found`,
		},
		{
			obj: map[string]interface{}{
				"invalid": "{{res2#a.string}}",
			},
			err: `invalid reference at "invalid": object not found: res2#a.string`,
		},
		{
			obj: map[string]interface{}{
				"invalid": "{{res1}}",
			},
			err: `invalid reference at "invalid": cannot include whole object: res1`,
		},
		{
			obj: map[string]interface{}{
				"invalid": "{{res1}}",
			},
			err: `invalid reference at "invalid": cannot include whole object: res1`,
		},
		{
			obj: map[string]interface{}{
				"invalid": "{{self1#x.b}}",
			},
			err: `invalid reference at "invalid": self references are not allowed: self1#x.b`,
		},
		{
			obj: map[string]interface{}{
				"invalid": "{{self1#x.b}}",
			},
			err: `invalid reference at "invalid": self references are not allowed: self1#x.b`,
		},
		{
			obj: map[string]interface{}{
				"invalid": "{{resX#a.string}}",
			},
			err: `invalid reference at "invalid": references can only point at direct dependencies: resX#a.string`,
		},
		{
			obj: map[string]interface{}{
				"invalid": "{{res1:bindsecret#Data.password}}",
			},
			err: `invalid reference at "invalid": "bindsecret" requested, but "res1" is not a ServiceBinding`,
		},
		{
			obj: map[string]interface{}{
				"invalid": "{{resbinding:someotherthing#notthere}}",
			},
			err: `invalid reference at "invalid": resource output name "someotherthing" not understood for "resbinding"`,
		},
		{
			obj: map[string]interface{}{
				"invalid": "{{resbinding:bindsecret#Data.nonutf8}}",
			},
			err: `invalid reference at "invalid": cannot expand non-UTF8 byte array field "resbinding:bindsecret#Data.nonutf8"`,
		},
		{
			obj: map[string]interface{}{
				"invalid": "{{resbinding:bindsecret#Data.nonutf8}}",
			},
			err:          `invalid reference at "invalid": no default value provided in selector "resbinding:bindsecret#Data.nonutf8"`,
			defaultsOnly: true,
		},
		{
			obj: map[string]interface{}{
				"invalid": "{{resX#a.string}}",
			},
			err:          `invalid reference at "invalid": references can only point at direct dependencies: resX#a.string`,
			defaultsOnly: true,
		},
	}
	for i, input := range inputs {
		input := input
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()
			sp := SpecProcessor{
				selfName:  "self1",
				resources: processedResources(),
				allowedResources: map[smith_v1.ResourceName]struct{}{
					"res1":       {},
					"resbinding": {},
					"res2":       {},
				},
				defaultsOnly: input.defaultsOnly,
			}
			assert.EqualError(t, sp.ProcessObject(input.obj), input.err)
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
