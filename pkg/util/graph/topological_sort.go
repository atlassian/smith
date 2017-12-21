package graph

import "github.com/pkg/errors"

func (g *Graph) TopologicalSort() ([]V, error) {
	results := newOrderedSet()
	for _, name := range g.orderedVertices {
		err := g.visit(name, results, nil)
		if err != nil {
			return nil, err
		}
	}

	return results.items, nil
}

func (g *Graph) visit(name V, results *orderedset, visited *orderedset) error {
	if visited == nil {
		visited = newOrderedSet()
	}

	added := visited.add(name)
	if !added {
		index := visited.index(name)
		cycle := append(visited.items[index:], name)
		return errors.Errorf("cycle error: %v", cycle)
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
	indexes map[V]int
	items   []V
	length  int
}

func newOrderedSet() *orderedset {
	return &orderedset{
		indexes: make(map[V]int),
		length:  0,
	}
}

func (s *orderedset) add(item V) bool {
	_, ok := s.indexes[item]
	if ok {
		return false
	}
	s.indexes[item] = s.length
	s.items = append(s.items, item)
	s.length++
	return true
}

func (s *orderedset) clone() *orderedset {
	clone := newOrderedSet()
	for _, item := range s.items {
		clone.add(item)
	}
	return clone
}

func (s *orderedset) index(item V) int {
	index, ok := s.indexes[item]
	if ok {
		return index
	}
	return -1
}
