package controller

import (
	"testing"

	"github.com/atlassian/smith"
	"github.com/atlassian/smith/pkg/util/graph"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBundleSort(t *testing.T) {
	t.Parallel()
	bundle := smith.Bundle{
		Spec: smith.BundleSpec{
			Resources: []smith.Resource{
				{
					Name:      "a",
					DependsOn: []smith.ResourceName{"c"},
				},
				{
					Name: "b",
				},
				{
					Name:      "c",
					DependsOn: []smith.ResourceName{"b"},
				},
				{
					Name:      "d",
					DependsOn: []smith.ResourceName{"e"},
				},
				{
					Name: "e",
				},
			},
		},
	}
	_, sorted, err := sortBundle(&bundle)
	require.NoError(t, err)

	assert.EqualValues(t, []graph.V{smith.ResourceName("b"), smith.ResourceName("c"), smith.ResourceName("a"), smith.ResourceName("e"), smith.ResourceName("d")}, sorted)
}

func TestBundleSortMissingDependency(t *testing.T) {
	t.Parallel()
	bundle := smith.Bundle{
		Spec: smith.BundleSpec{
			Resources: []smith.Resource{
				{
					Name:      "a",
					DependsOn: []smith.ResourceName{"x"},
				},
				{
					Name: "b",
				},
				{
					Name:      "c",
					DependsOn: []smith.ResourceName{"b"},
				},
				{
					Name:      "d",
					DependsOn: []smith.ResourceName{"e"},
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
