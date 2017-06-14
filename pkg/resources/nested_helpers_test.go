package resources

import (
	"testing"

	"encoding/json"

	"github.com/atlassian/smith"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetJsonPathStringBundle(t *testing.T) {
	b := &smith.Bundle{
		Status: smith.BundleStatus{
			Conditions: []smith.BundleCondition{
				{
					Type:   smith.BundleError,
					Status: smith.ConditionFalse,
				},
				{
					Type:   smith.BundleReady,
					Status: smith.ConditionTrue,
				},
				{
					Type:   smith.BundleInProgress,
					Status: smith.ConditionFalse,
				},
			},
		},
	}
	bytes, err := json.Marshal(b)
	require.NoError(t, err)
	unstructured := make(map[string]interface{})
	err = json.Unmarshal(bytes, &unstructured)
	require.NoError(t, err)
	status, err := GetJsonPathString(unstructured, `{$.status.conditions[?(@.type=="Ready")].status}`)
	require.NoError(t, err)
	assert.Equal(t, string(smith.ConditionTrue), status)
}

func TestGetJsonPathStringMissing(t *testing.T) {
	// Bundle with empty status
	b := &smith.Bundle{}
	bytes, err := json.Marshal(b)
	require.NoError(t, err)
	unstructured := make(map[string]interface{})
	err = json.Unmarshal(bytes, &unstructured)
	require.NoError(t, err)
	status, err := GetJsonPathString(unstructured, `{$.status.conditions[?(@.type=="Ready")].status}`)
	// Should return empty string without errors
	require.NoError(t, err)
	require.Equal(t, "", status)
}

func TestGetJsonPathStringInvalid(t *testing.T) {
	b := &smith.Bundle{
		Status: smith.BundleStatus{
			Conditions: []smith.BundleCondition{
				{
					Type:   smith.BundleReady,
					Status: smith.ConditionTrue,
				},
			},
		},
	}
	bytes, err := json.Marshal(b)
	require.NoError(t, err)
	unstructured := make(map[string]interface{})
	err = json.Unmarshal(bytes, &unstructured)
	require.NoError(t, err)
	// Invalid JsonPath format: missing quotes around "Ready"
	_, err = GetJsonPathString(unstructured, `{$.status.conditions[?(@.type==Ready)].status}`)
	require.Error(t, err)
	require.Equal(t, "JsonPath execute error unrecognized identifier Ready", err.Error())
}
