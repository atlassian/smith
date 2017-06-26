package controller

import (
	"fmt"
	"testing"

	"github.com/atlassian/smith/pkg/cleanup"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestUpdateResourceEmptyMissingNilNoChanges(t *testing.T) {
	t.Parallel()

	inputs := map[string]func() *unstructured.Unstructured{
		"empty":   emptyMap,
		"missing": missingMap,
		"nil":     nilMap,
	}

	for kind1, input1 := range inputs {
		for kind2, input2 := range inputs {
			actual := input1()
			desired := input2()
			t.Run(fmt.Sprintf("%s actual, %s desired", kind1, kind2), func(t *testing.T) {
				t.Parallel()
				cntrlr := BundleController{
					scheme:  runtime.NewScheme(),
					cleaner: cleanup.New(),
				}
				updated, match, err := cntrlr.compareActualVsSpec(desired, actual)
				require.NoError(t, err)
				assert.True(t, match)
				assert.Equal(t, actual, updated)
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
				"ownerReferences": []map[string]interface{}{},
				"finalizers":      []string{},
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
