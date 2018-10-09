package graph

import "github.com/pkg/errors"

// V is name of the vertex.
type V interface{}

// D is data attached to the vertex.
type D interface{}

// Vertex is a resource representation in a dependency graph.
type Vertex struct {
	// Edges in order of appearance (for deterministic order after sort).
	OutgoingEdges []V
	// Edges in the inverse of outgoing, i.e. vertices that depend on this vertex
	IncomingEdges []V
	Data          D
}

// Graph is a graph representation of resource dependencies.
type Graph struct {
	// Vertices is a map from resource name to resource vertex.
	Vertices map[V]*Vertex

	// Vertices in order of appearance (for deterministic order after sort).
	orderedVertices []V
}

func NewGraph(size int) *Graph {
	return &Graph{
		Vertices:        make(map[V]*Vertex, size),
		orderedVertices: make([]V, 0, size),
	}
}

func (g *Graph) AddVertex(name V, data D) {
	if !g.ContainsVertex(name) {
		g.Vertices[name] = &Vertex{
			Data: data,
		}
		g.orderedVertices = append(g.orderedVertices, name)
	}
}

func (g *Graph) AddEdge(from, to V) error {
	f, ok := g.Vertices[from]
	if !ok {
		return errors.Errorf("vertex %q not found", from)
	}
	t, ok := g.Vertices[to]
	if !ok {
		return errors.Errorf("vertex %q not found", to)
	}

	f.addOutgoingEdge(to)
	t.addIncomingEdge(from)
	return nil
}

func (g *Graph) ContainsVertex(name V) bool {
	_, ok := g.Vertices[name]
	return ok
}

func (v *Vertex) addOutgoingEdge(name V) {
	v.OutgoingEdges = append(v.OutgoingEdges, name)
}

func (v *Vertex) addIncomingEdge(name V) {
	v.IncomingEdges = append(v.IncomingEdges, name)
}
