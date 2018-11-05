package testing

import (
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type FakeStore struct {
	Namespace string
	Responses map[string]runtime.Object
}

func (f FakeStore) Get(gvk schema.GroupVersionKind, namespace, name string) (runtime.Object, bool /*exists*/, error) {
	if f.Namespace != namespace {
		return nil, false, errors.Errorf("namespace does not match: expected %q != passed %q", f.Namespace, namespace)
	}
	v, ok := f.Responses[name]
	return v, ok, nil
}
