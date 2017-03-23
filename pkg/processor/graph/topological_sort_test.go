package graph

import (
	"testing"

	"github.com/atlassian/smith"

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
					DependsOn: []smith.DependencyRef{"c"},
				},
				{
					Name: "b",
				},
				{
					Name:      "c",
					DependsOn: []smith.DependencyRef{"b"},
				},
				{
					Name:      "d",
					DependsOn: []smith.DependencyRef{"e"},
				},
				{
					Name: "e",
				},
			},
		},
	}
	graphData, err := TopologicalSort(&bundle)
	require.NoError(t, err)

	assert.Equal(t, []string{"b", "c", "a", "e", "d"}, graphData.SortedVertices)
}

func TestSort1(t *testing.T) {
	t.Parallel()
	graph := initGraph()

	// a -> b -> c
	require.NoError(t, graph.addEdge("a", "b"))
	require.NoError(t, graph.addEdge("b", "c"))

	assertSortResult(t, graph, []string{"c", "b", "a", "d"})
}

func TestSort2(t *testing.T) {
	t.Parallel()
	graph := initGraph()

	// a -> c
	// a -> b
	// b -> c
	require.NoError(t, graph.addEdge("a", "c"))
	require.NoError(t, graph.addEdge("a", "b"))
	require.NoError(t, graph.addEdge("b", "c"))

	assertSortResult(t, graph, []string{"c", "b", "a", "d"})
}

func TestSort3(t *testing.T) {
	t.Parallel()
	graph := initGraph()

	// a -> b
	// a -> d
	// d -> c
	// c -> b
	require.NoError(t, graph.addEdge("a", "b"))
	require.NoError(t, graph.addEdge("a", "d"))
	require.NoError(t, graph.addEdge("d", "c"))
	require.NoError(t, graph.addEdge("c", "b"))

	assertSortResult(t, graph, []string{"b", "c", "d", "a"})
}

func TestSortCycleError1(t *testing.T) {
	t.Parallel()
	graph := initGraph()

	// a -> b
	// b -> a
	require.NoError(t, graph.addEdge("a", "b"))
	require.NoError(t, graph.addEdge("b", "a"))

	assertCycleDetection(t, graph)
}

func TestSortCycleError2(t *testing.T) {
	t.Parallel()
	graph := initGraph()

	// a -> b
	// b -> c
	// c -> a
	require.NoError(t, graph.addEdge("a", "b"))
	require.NoError(t, graph.addEdge("b", "c"))
	require.NoError(t, graph.addEdge("c", "a"))

	assertCycleDetection(t, graph)
}

func TestSortCycleError3(t *testing.T) {
	t.Parallel()
	graph := initGraph()

	// a -> b
	// b -> c
	// c -> b
	require.NoError(t, graph.addEdge("a", "b"))
	require.NoError(t, graph.addEdge("b", "c"))
	require.NoError(t, graph.addEdge("c", "b"))

	assertCycleDetection(t, graph)
}

func initGraph() *Graph {
	graph := newGraph(4)
	graph.addVertex("a")
	graph.addVertex("b")
	graph.addVertex("c")
	graph.addVertex("d")
	return graph
}

func assertSortResult(t *testing.T, graph *Graph, expected []string) {
	result, err := graph.topologicalSort()
	require.NoError(t, err)
	assert.Equal(t, expected, result)
}

func assertCycleDetection(t *testing.T, graph *Graph) {
	_, err := graph.topologicalSort()
	require.Error(t, err)
}
