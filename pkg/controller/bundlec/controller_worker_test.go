package bundlec

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
					Name: "a",
					References: []smith_v1.Reference{
						{
							Resource: "c",
						},
					},
				},
				{
					Name: "b",
				},
				{
					Name: "c",
					References: []smith_v1.Reference{
						{
							Resource: "b",
						},
					},
				},
				{
					Name: "d",
					References: []smith_v1.Reference{
						{
							Resource: "e",
						},
					},
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
					Name: "a",
					References: []smith_v1.Reference{
						{
							Resource: "x",
						},
					},
				},
				{
					Name: "b",
				},
				{
					Name: "c",
					References: []smith_v1.Reference{
						{
							Resource: "b",
						},
					},
				},
				{
					Name: "d",
					References: []smith_v1.Reference{
						{
							Resource: "e",
						},
					},
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

func TestBundleSortSelfReference(t *testing.T) {
	t.Parallel()
	bundle := smith_v1.Bundle{
		Spec: smith_v1.BundleSpec{
			Resources: []smith_v1.Resource{
				{
					Name: "a",
					References: []smith_v1.Reference{
						{
							Resource: "a",
						},
					},
				},
				{
					Name: "b",
				},
				{
					Name: "c",
					References: []smith_v1.Reference{
						{
							Resource: "b",
						},
					},
				},
				{
					Name: "d",
					References: []smith_v1.Reference{
						{
							Resource: "e",
						},
					},
				},
				{
					Name: "e",
				},
			},
		},
	}
	_, sorted, err := sortBundle(&bundle)
	require.EqualError(t, err, "cycle error: [a a]", "%v", sorted)
}
