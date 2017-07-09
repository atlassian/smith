package graph

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSort1(t *testing.T) {
	t.Parallel()
	g := initGraph()

	// a -> b -> c
	require.NoError(t, g.AddEdge("a", "b"))
	require.NoError(t, g.AddEdge("b", "c"))

	assertSortResult(t, g, []V{"c", "b", "a", "d"})
}

func TestSort2(t *testing.T) {
	t.Parallel()
	g := initGraph()

	// a -> c
	// a -> b
	// b -> c
	require.NoError(t, g.AddEdge("a", "c"))
	require.NoError(t, g.AddEdge("a", "b"))
	require.NoError(t, g.AddEdge("b", "c"))

	assertSortResult(t, g, []V{"c", "b", "a", "d"})
}

func TestSort3(t *testing.T) {
	t.Parallel()
	g := initGraph()

	// a -> b
	// a -> d
	// d -> c
	// c -> b
	require.NoError(t, g.AddEdge("a", "b"))
	require.NoError(t, g.AddEdge("a", "d"))
	require.NoError(t, g.AddEdge("d", "c"))
	require.NoError(t, g.AddEdge("c", "b"))

	assertSortResult(t, g, []V{"b", "c", "d", "a"})
}

func TestSortCycleError1(t *testing.T) {
	t.Parallel()
	g := initGraph()

	// a -> b
	// b -> a
	require.NoError(t, g.AddEdge("a", "b"))
	require.NoError(t, g.AddEdge("b", "a"))

	assertCycleDetection(t, g)
}

func TestSortCycleError2(t *testing.T) {
	t.Parallel()
	g := initGraph()

	// a -> b
	// b -> c
	// c -> a
	require.NoError(t, g.AddEdge("a", "b"))
	require.NoError(t, g.AddEdge("b", "c"))
	require.NoError(t, g.AddEdge("c", "a"))

	assertCycleDetection(t, g)
}

func TestSortCycleError3(t *testing.T) {
	t.Parallel()
	g := initGraph()

	// a -> b
	// b -> c
	// c -> b
	require.NoError(t, g.AddEdge("a", "b"))
	require.NoError(t, g.AddEdge("b", "c"))
	require.NoError(t, g.AddEdge("c", "b"))

	assertCycleDetection(t, g)
}

func initGraph() *Graph {
	g := NewGraph(4)
	g.AddVertex("a", 1)
	g.AddVertex("b", 2)
	g.AddVertex("c", 3)
	g.AddVertex("d", 4)
	return g
}

func assertSortResult(t *testing.T, g *Graph, expected []V) {
	result, err := g.TopologicalSort()
	require.NoError(t, err)
	assert.Equal(t, expected, result)
}

func assertCycleDetection(t *testing.T, g *Graph) {
	_, err := g.TopologicalSort()
	require.Error(t, err)
}
