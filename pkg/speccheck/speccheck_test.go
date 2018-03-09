package speccheck

import (
	"fmt"
	"testing"

	"github.com/atlassian/smith/pkg/cleanup"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestUpdateResourceEmptyMissingNilNoChanges(t *testing.T) {
	t.Parallel()

	inputs := map[string]func() *unstructured.Unstructured{
		"empty":   emptyMap,
		"missing": missingMap,
		"nil":     nilMap,
	}
	loggerConfig := zap.NewDevelopmentConfig()
	loggerConfig.DisableCaller = true
	logger, err := loggerConfig.Build()
	require.NoError(t, err)
	defer logger.Sync()

	for kind1, input1 := range inputs {
		for kind2, input2 := range inputs {
			actual := input1()
			spec := input2()
			t.Run(fmt.Sprintf("%s actual, %s spec", kind1, kind2), func(t *testing.T) {
				t.Parallel()
				sc := SpecCheck{
					Logger:  logger,
					Cleaner: cleanup.New(),
				}
				updated, match, err := sc.CompareActualVsSpec(spec, actual)
				require.NoError(t, err)
				assert.True(t, match)
				assert.True(t, equality.Semantic.DeepEqual(updated.Object, actual.Object))
			})
		}
	}
}

func emptyMap() *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":            "map1",
				"annotations":     map[string]interface{}{},
				"labels":          map[string]interface{}{},
				"ownerReferences": []interface{}{},
				"finalizers":      []interface{}{},
			},
			"data": map[string]interface{}{
				"a": "b",
			},
		},
	}
}

func missingMap() *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name": "map1",
			},
			"data": map[string]interface{}{
				"a": "b",
			},
		},
	}
}

func nilMap() *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":            "map1",
				"annotations":     nil,
				"labels":          nil,
				"ownerReferences": nil,
				"finalizers":      nil,
			},
			"data": map[string]interface{}{
				"a": "b",
			},
		},
	}
}
