package graph

import (
	"fmt"
	"log"
	"strings"

	"github.com/atlassian/smith"
)

// SortedData is a container for dependency graph and topological sort result
type SortedData struct {
	Graph          *Graph
	SortedVertices []string
}

// TopologicalSort builds resource dependency graph and topologically sorts it
func TopologicalSort(bundle *smith.Bundle) (*SortedData, error) {
	graph := newGraph(len(bundle.Spec.Resources))

	for _, res := range bundle.Spec.Resources {
		graph.addVertex(res.Name)
	}

	for _, res := range bundle.Spec.Resources {
		for _, d := range res.DependsOn {
			graph.addEdge(res.Name, string(d))
		}
	}

	sorted, err := graph.topologicalSort()
	if err != nil {
		return nil, err
	}

	graphData := SortedData{
		Graph:          graph,
		SortedVertices: sorted,
	}

	log.Printf("Sorted graph: %v", sorted)

	return &graphData, nil
}

func (g *Graph) topologicalSort() ([]string, error) {
	results := newOrderedSet()
	for _, name := range g.orderedVertices {
		err := g.visit(name, results, nil)
		if err != nil {
			return nil, err
		}
	}

	return results.items, nil
}

func (g *Graph) visit(name string, results *orderedset, visited *orderedset) error {
	if visited == nil {
		visited = newOrderedSet()
	}

	added := visited.add(name)
	if !added {
		index := visited.index(name)
		cycle := append(visited.items[index:], name)
		return fmt.Errorf("Cycle error: %s", strings.Join(cycle, " -> "))
	}

	n := g.Vertices[name]
	for _, edge := range n.Edges() {
		err := g.visit(edge, results, visited.clone())
		if err != nil {
			return err
		}
	}

	results.add(name)
	return nil
}

type orderedset struct {
	indexes map[string]int
	items   []string
	length  int
}

func newOrderedSet() *orderedset {
	return &orderedset{
		indexes: make(map[string]int),
		length:  0,
	}
}

func (s *orderedset) add(item string) bool {
	_, ok := s.indexes[item]
	if !ok {
		s.indexes[item] = s.length
		s.items = append(s.items, item)
		s.length++
	}
	return !ok
}

func (s *orderedset) clone() *orderedset {
	clone := newOrderedSet()
	for _, item := range s.items {
		clone.add(item)
	}
	return clone
}

func (s *orderedset) index(item string) int {
	index, ok := s.indexes[item]
	if ok {
		return index
	}
	return -1
}
