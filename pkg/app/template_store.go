package app

import (
	"github.com/atlassian/smith"

	"k8s.io/client-go/tools/cache"
)

type templateStore struct {
	store cache.Store
}

func (s *templateStore) Get(namespace, tmplName string) (*smith.Template, error) {
	tmpl, exists, err := s.store.GetByKey(keyForTemplate(namespace, tmplName))
	if err != nil || !exists {
		return nil, err
	}
	in := tmpl.(*smith.Template)
	out := &smith.Template{}

	if err := smith.DeepCopy_Template(in, out); err != nil {
		return nil, err
	}
	return out, nil
}

// keyForTemplate returns same key as client-go/tools/cache/store.MetaNamespaceKeyFunc
func keyForTemplate(namespace, tmplName string) string {
	return namespace + "/" + tmplName
}
