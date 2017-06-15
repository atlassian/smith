package tprattribute

import (
	"encoding/json"
	"testing"

	"github.com/atlassian/smith/pkg/resources"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var _ runtime.Object = &SleeperList{}
var _ meta_v1.ListMetaAccessor = &SleeperList{}

var _ runtime.Object = &Sleeper{}
var _ meta_v1.ObjectMetaAccessor = &Sleeper{}

func TestGetJsonPathStringSleeper(t *testing.T) {
	t.Parallel()
	b := &Sleeper{
		Status: SleeperStatus{
			State: Awake,
		},
	}
	bytes, err := json.Marshal(b)
	require.NoError(t, err)
	unstructured := make(map[string]interface{})
	err = json.Unmarshal(bytes, &unstructured)
	require.NoError(t, err)
	status, err := resources.GetJsonPathString(unstructured, SleeperReadyStatePath)
	require.NoError(t, err)
	assert.Equal(t, string(SleeperReadyStateValue), status)
}
