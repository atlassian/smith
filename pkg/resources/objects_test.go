package resources

import (
	"testing"

	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/util/json"
)

func TestGetJsonPathStringBundle(t *testing.T) {
	t.Parallel()
	b := &smith_v1.Bundle{
		Status: smith_v1.BundleStatus{
			Conditions: []smith_v1.BundleCondition{
				{
					Type:   smith_v1.BundleError,
					Status: smith_v1.ConditionFalse,
				},
				{
					Type:   smith_v1.BundleReady,
					Status: smith_v1.ConditionTrue,
				},
				{
					Type:   smith_v1.BundleInProgress,
					Status: smith_v1.ConditionFalse,
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
	assert.Equal(t, string(smith_v1.ConditionTrue), status)
}

func TestGetJsonPathStringMissing(t *testing.T) {
	t.Parallel()
	// Bundle with empty status
	b := &smith_v1.Bundle{}
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
	t.Parallel()
	b := &smith_v1.Bundle{
		Status: smith_v1.BundleStatus{
			Conditions: []smith_v1.BundleCondition{
				{
					Type:   smith_v1.BundleReady,
					Status: smith_v1.ConditionTrue,
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
	require.EqualError(t, err, "JsonPath execute error: unrecognized identifier Ready")
}
