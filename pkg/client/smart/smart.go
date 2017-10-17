package smart

import (
	"fmt"

	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	scClientset "github.com/kubernetes-incubator/service-catalog/pkg/client/clientset_generated/clientset"
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
	CoreDynamic, ScDynamic ClientPool
	Mapper, ScMapper       Mapper
}

func NewClient(config, scConfig *rest.Config, clientset kubernetes.Interface, scClient scClientset.Interface) *DynamicClient {
	rm := discovery.NewDeferredDiscoveryRESTMapper(
		&CachedDiscoveryClient{
			DiscoveryInterface: clientset.Discovery(),
		},
		meta.InterfacesForUnstructured,
	)
	var scRm meta.RESTMapper
	var scDynamic dynamic.ClientPool
	if scClient != nil {
		scRm = discovery.NewDeferredDiscoveryRESTMapper(
			&CachedDiscoveryClient{
				DiscoveryInterface: scClient.Discovery(),
			},
			meta.InterfacesForUnstructured,
		)
		scDynamic = dynamic.NewClientPool(scConfig, scRm, dynamic.LegacyAPIPathResolverFunc)
	}

	return &DynamicClient{
		CoreDynamic: dynamic.NewClientPool(config, rm, dynamic.LegacyAPIPathResolverFunc),
		ScDynamic:   scDynamic,
		Mapper:      rm,
		ScMapper:    scRm,
	}
}

func (c *DynamicClient) ForGVK(gvk schema.GroupVersionKind, namespace string) (dynamic.ResourceInterface, error) {
	var clients ClientPool
	var m Mapper
	if gvk.Group == sc_v1b1.GroupName {
		if c.ScDynamic == nil {
			return nil, fmt.Errorf("client for Service Catalog is not configured, cannot work with object %s", gvk)
		}
		clients = c.ScDynamic
		m = c.ScMapper
	} else {
		clients = c.CoreDynamic
		m = c.Mapper
	}
	client, err := clients.ClientForGroupVersionKind(gvk)
	if err != nil {
		return nil, fmt.Errorf("failed to instantiate client for %v: %v", gvk, err)
	}
	rm, err := m.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return nil, fmt.Errorf("failed to get rest mapping for %v: %v", gvk, err)
	}

	return client.Resource(&meta_v1.APIResource{
		Name:       rm.Resource,
		Namespaced: namespace != meta_v1.NamespaceNone,
		Kind:       gvk.Kind,
	}, namespace), nil
}
