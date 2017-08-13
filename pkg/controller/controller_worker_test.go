package controller

import (
	"testing"

	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/smith/pkg/util/graph"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBundleSort(t *testing.T) {
	t.Parallel()
	bundle := smith_v1.Bundle{
		Spec: smith_v1.BundleSpec{
			Resources: []smith_v1.Resource{
				{
					Name:      "a",
					DependsOn: []smith_v1.ResourceName{"c"},
				},
				{
					Name: "b",
				},
				{
					Name:      "c",
					DependsOn: []smith_v1.ResourceName{"b"},
				},
				{
					Name:      "d",
					DependsOn: []smith_v1.ResourceName{"e"},
				},
				{
					Name: "e",
				},
			},
		},
	}
	_, sorted, err := sortBundle(&bundle)
	require.NoError(t, err)

	assert.EqualValues(t, []graph.V{smith_v1.ResourceName("b"), smith_v1.ResourceName("c"), smith_v1.ResourceName("a"), smith_v1.ResourceName("e"), smith_v1.ResourceName("d")}, sorted)
}

func TestBundleSortMissingDependency(t *testing.T) {
	t.Parallel()
	bundle := smith_v1.Bundle{
		Spec: smith_v1.BundleSpec{
			Resources: []smith_v1.Resource{
				{
					Name:      "a",
					DependsOn: []smith_v1.ResourceName{"x"},
				},
				{
					Name: "b",
				},
				{
					Name:      "c",
					DependsOn: []smith_v1.ResourceName{"b"},
				},
				{
					Name:      "d",
					DependsOn: []smith_v1.ResourceName{"e"},
				},
				{
					Name: "e",
				},
			},
		},
	}
	_, sorted, err := sortBundle(&bundle)
	require.EqualError(t, err, "vertex \"x\" not found", "%v", sorted)
}
