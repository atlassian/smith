package smart

import (
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

type DynamicClient struct {
	DynamicClient dynamic.Interface
	RESTMapper    meta.RESTMapper
}

func (c *DynamicClient) ForGVK(gvk schema.GroupVersionKind, namespace string) (dynamic.ResourceInterface, error) {
	rm, err := c.RESTMapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get rest mapping for %s", gvk)
	}
	return c.DynamicClient.Resource(rm.Resource).Namespace(namespace), nil
}
