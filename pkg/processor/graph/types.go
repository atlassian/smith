package graph

import (
	"fmt"

	"github.com/atlassian/smith"
)

// Graph is a graph representation of resource dependencies
type Graph struct {
	// Vertices is a map from resource name to resource vertex
	Vertices map[smith.ResourceName]Vertex

	// Vertices in order of appearance (for deterministic order after sort)
	orderedVertices []smith.ResourceName
}

// Vertex is a resource representation in a dependency graph
type Vertex map[smith.ResourceName]struct{}

func newGraph(size int) *Graph {
	return &Graph{
		Vertices:        make(map[smith.ResourceName]Vertex, size),
		orderedVertices: make([]smith.ResourceName, 0, size),
	}
}

func (g *Graph) addVertex(name smith.ResourceName) {
	if !g.containsVertex(name) {
		g.Vertices[name] = make(Vertex)
		g.orderedVertices = append(g.orderedVertices, name)
	}
}

func (g *Graph) addEdge(from, to smith.ResourceName) error {
	f, ok := g.Vertices[from]
	if !ok {
		return fmt.Errorf("vertex %q not found", from)
	}
	_, ok = g.Vertices[to]
	if !ok {
		return fmt.Errorf("vertex %q not found", to)
	}

	f.addEdge(to)
	return nil
}

func (g *Graph) containsVertex(name smith.ResourceName) bool {
	_, ok := g.Vertices[name]
	return ok
}

func (v Vertex) addEdge(name smith.ResourceName) {
	v[name] = struct{}{}
}

// Edges returns a list of resources current resource depends on
func (v Vertex) Edges() []smith.ResourceName {
	keys := make([]smith.ResourceName, 0, len(v))
	for k := range v {
		keys = append(keys, k)
	}
	return keys
}
