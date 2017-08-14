// Generated file, do not modify manually!
package v1

import (
	v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/smith/pkg/client/clientset_generated/clientset/scheme"
	serializer "k8s.io/apimachinery/pkg/runtime/serializer"
	rest "k8s.io/client-go/rest"
)

type SmithV1Interface interface {
	RESTClient() rest.Interface
	BundlesGetter
}

// SmithV1Client is used to interact with features provided by the smith.atlassian.com group.
type SmithV1Client struct {
	restClient rest.Interface
}

func (c *SmithV1Client) Bundles(namespace string) BundleInterface {
	return newBundles(c, namespace)
}

// NewForConfig creates a new SmithV1Client for the given config.
func NewForConfig(c *rest.Config) (*SmithV1Client, error) {
	config := *c
	if err := setConfigDefaults(&config); err != nil {
		return nil, err
	}
	client, err := rest.RESTClientFor(&config)
	if err != nil {
		return nil, err
	}
	return &SmithV1Client{client}, nil
}

// NewForConfigOrDie creates a new SmithV1Client for the given config and
// panics if there is an error in the config.
func NewForConfigOrDie(c *rest.Config) *SmithV1Client {
	client, err := NewForConfig(c)
	if err != nil {
		panic(err)
	}
	return client
}

// New creates a new SmithV1Client for the given RESTClient.
func New(c rest.Interface) *SmithV1Client {
	return &SmithV1Client{c}
}

func setConfigDefaults(config *rest.Config) error {
	gv := v1.SchemeGroupVersion
	config.GroupVersion = &gv
	config.APIPath = "/apis"
	config.NegotiatedSerializer = serializer.DirectCodecFactory{CodecFactory: scheme.Codecs}

	if config.UserAgent == "" {
		config.UserAgent = rest.DefaultKubernetesUserAgent()
	}

	return nil
}

// RESTClient returns a RESTClient that is used to communicate
// with API server by this client implementation.
func (c *SmithV1Client) RESTClient() rest.Interface {
	if c == nil {
		return nil
	}
	return c.restClient
}
