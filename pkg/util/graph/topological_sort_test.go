package graph

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSort1(t *testing.T) {
	t.Parallel()
	graph := initGraph()

	// a -> b -> c
	require.NoError(t, graph.AddEdge("a", "b"))
	require.NoError(t, graph.AddEdge("b", "c"))

	assertSortResult(t, graph, []V{"c", "b", "a", "d"})
}

func TestSort2(t *testing.T) {
	t.Parallel()
	graph := initGraph()

	// a -> c
	// a -> b
	// b -> c
	require.NoError(t, graph.AddEdge("a", "c"))
	require.NoError(t, graph.AddEdge("a", "b"))
	require.NoError(t, graph.AddEdge("b", "c"))

	assertSortResult(t, graph, []V{"c", "b", "a", "d"})
}

func TestSort3(t *testing.T) {
	t.Parallel()
	graph := initGraph()

	// a -> b
	// a -> d
	// d -> c
	// c -> b
	require.NoError(t, graph.AddEdge("a", "b"))
	require.NoError(t, graph.AddEdge("a", "d"))
	require.NoError(t, graph.AddEdge("d", "c"))
	require.NoError(t, graph.AddEdge("c", "b"))

	assertSortResult(t, graph, []V{"b", "c", "d", "a"})
}

func TestSortCycleError1(t *testing.T) {
	t.Parallel()
	graph := initGraph()

	// a -> b
	// b -> a
	require.NoError(t, graph.AddEdge("a", "b"))
	require.NoError(t, graph.AddEdge("b", "a"))

	assertCycleDetection(t, graph)
}

func TestSortCycleError2(t *testing.T) {
	t.Parallel()
	graph := initGraph()

	// a -> b
	// b -> c
	// c -> a
	require.NoError(t, graph.AddEdge("a", "b"))
	require.NoError(t, graph.AddEdge("b", "c"))
	require.NoError(t, graph.AddEdge("c", "a"))

	assertCycleDetection(t, graph)
}

func TestSortCycleError3(t *testing.T) {
	t.Parallel()
	graph := initGraph()

	// a -> b
	// b -> c
	// c -> b
	require.NoError(t, graph.AddEdge("a", "b"))
	require.NoError(t, graph.AddEdge("b", "c"))
	require.NoError(t, graph.AddEdge("c", "b"))

	assertCycleDetection(t, graph)
}

func initGraph() *Graph {
	graph := NewGraph(4)
	graph.AddVertex("a", 1)
	graph.AddVertex("b", 2)
	graph.AddVertex("c", 3)
	graph.AddVertex("d", 4)
	return graph
}

func assertSortResult(t *testing.T, graph *Graph, expected []V) {
	result, err := graph.TopologicalSort()
	require.NoError(t, err)
	assert.Equal(t, expected, result)
}

func assertCycleDetection(t *testing.T, graph *Graph) {
	_, err := graph.TopologicalSort()
	require.Error(t, err)
}
