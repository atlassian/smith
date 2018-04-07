package smart

import (
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type Mapper interface {
	// RESTMapping identifies a preferred resource mapping for the provided group kind.
	RESTMapping(gk schema.GroupKind, versions ...string) (*meta.RESTMapping, error)
}

// ClientPool manages a pool of dynamic clients.
type ClientPool interface {
	// ClientForGroupVersionKind returns a client configured for the specified groupVersionKind.
	// Kind may be empty.
	ClientForGroupVersionKind(schema.GroupVersionKind) (dynamic.Interface, error)
}

type DynamicClient struct {
	ClientPool ClientPool
	Mapper     Mapper
}

func NewClient(config *rest.Config, mainClient kubernetes.Interface) *DynamicClient {
	rm := discovery.NewDeferredDiscoveryRESTMapper(
		&CachedDiscoveryClient{
			DiscoveryInterface: mainClient.Discovery(),
		},
		meta.InterfacesForUnstructured,
	)
	return &DynamicClient{
		ClientPool: dynamic.NewClientPool(config, rm, dynamic.LegacyAPIPathResolverFunc),
		Mapper:     rm,
	}
}

func (c *DynamicClient) ForGVK(gvk schema.GroupVersionKind, namespace string) (dynamic.ResourceInterface, error) {
	client, err := c.ClientPool.ClientForGroupVersionKind(gvk)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to instantiate client for %s", gvk)
	}
	rm, err := c.Mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get rest mapping for %s", gvk)
	}

	return client.Resource(&meta_v1.APIResource{
		Name:       rm.Resource,
		Namespaced: namespace != meta_v1.NamespaceNone,
		Kind:       gvk.Kind,
	}, namespace), nil
}
