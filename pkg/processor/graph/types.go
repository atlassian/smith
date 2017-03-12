package graph

import "fmt"

// Graph is a graph representation of resource dependencies
type Graph struct {
	// Vertices is a map from resource name to resource vertex
	Vertices map[string]Vertex

	// Vertices in order of appearance (for deterministic order after sort)
	orderedVertices []string
}

// Vertex is a resource representation in a dependency graph
type Vertex map[string]struct{}

func newGraph(size int) *Graph {
	return &Graph{
		Vertices:        make(map[string]Vertex, size),
		orderedVertices: make([]string, 0, size),
	}
}

func (g *Graph) addVertex(name string) {
	if !g.containsVertex(name) {
		g.Vertices[name] = make(Vertex)
		g.orderedVertices = append(g.orderedVertices, name)
	}
}

func (g *Graph) addEdge(from string, to string) error {
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

func (g *Graph) containsVertex(name string) bool {
	_, ok := g.Vertices[name]
	return ok
}

func (v Vertex) addEdge(name string) {
	v[name] = struct{}{}
}

// Edges returns a list of resources current resource depends on
func (v Vertex) Edges() []string {
	keys := make([]string, 0, len(v))
	for k := range v {
		keys = append(keys, k)
	}
	return keys
}
