package graph

import (
	"fmt"
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

func TestSortMissingVertexError(t *testing.T) {
	t.Parallel()
	g := initGraph()

	require.Error(t, g.AddEdge("a", "x"), "vertex \"x\" not found")
}

func TestSortIsDeterministic(t *testing.T) {
	t.Parallel()

	for n := 0; n <= 100; n++ {
		t.Run(fmt.Sprintf("Case%v", n), sortIsDeterministic)
	}
}

func TestInverseDependencies(t *testing.T) {
	t.Parallel()
	g := initGraph()

	// a -> b
	// b -> c
	// c -> d
	require.NoError(t, g.AddEdge("a", "b"))
	require.NoError(t, g.AddEdge("b", "c"))
	require.NoError(t, g.AddEdge("c", "d"))

	// ensure they are sorted in the correct order
	// d -> c -> b -> a
	sorted, err := g.TopologicalSort()
	require.NoError(t, err)
	assert.Equal(t, []V{"d", "c", "b", "a"}, sorted)

	// ensure we can trace in the opposite direction
	assert.Equal(t, V("a"), followIncomingEdges(sorted[0], g.Vertices))
}

func TestInverseDependencies2(t *testing.T) {
	t.Parallel()
	g := initGraph()

	// a -> b
	// b -> c
	// c -> b
	require.NoError(t, g.AddEdge("a", "c"))
	require.NoError(t, g.AddEdge("a", "b"))
	require.NoError(t, g.AddEdge("b", "c"))

	// ensure they are sorted in the correct order
	// c -> b -> a -> d
	sorted, err := g.TopologicalSort()
	require.NoError(t, err)
	assert.Equal(t, []V{"c", "b", "a", "d"}, sorted)

	// ensure we can trace in the opposite direction
	assert.Equal(t, V("a"), followIncomingEdges(sorted[0], g.Vertices))
	assert.Equal(t, V("a"), followIncomingEdges(sorted[1], g.Vertices))
	assert.Equal(t, V("a"), followIncomingEdges(sorted[2], g.Vertices))
	// d doesn't have any incoming edges, it returns itself
	assert.Equal(t, V("d"), followIncomingEdges(sorted[3], g.Vertices))
}

func TestInverseDependencies3(t *testing.T) {
	t.Parallel()
	g := initGraph()

	// a -> d
	// b -> d
	// c -> d
	require.NoError(t, g.AddEdge("a", "d"))
	require.NoError(t, g.AddEdge("b", "d"))
	require.NoError(t, g.AddEdge("c", "d"))

	// ensure they are sorted in the correct order
	// c -> a -> b -> c
	sorted, err := g.TopologicalSort()
	require.NoError(t, err)
	assert.Equal(t, []V{"d", "a", "b", "c"}, sorted)

	// ensure d has incoming edges referring to the 3 vertices that depend on it
	assert.Len(t, g.Vertices["d"].IncomingEdges, 3)
}

func followIncomingEdges(name V, vertices map[V]*Vertex) V {
	v := vertices[name]
	if len(v.IncomingEdges) == 0 {
		return name
	}
	return followIncomingEdges(v.IncomingEdges[0], vertices)
}

func sortIsDeterministic(t *testing.T) {
	g := initGraph()

	// a -> b
	// a -> c
	// a -> d
	require.NoError(t, g.AddEdge("a", "b"))
	require.NoError(t, g.AddEdge("a", "c"))
	require.NoError(t, g.AddEdge("a", "d"))

	assertSortResult(t, g, []V{"b", "c", "d", "a"})
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
