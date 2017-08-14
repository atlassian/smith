// Generated file, do not modify manually!
package fake

import (
	v1 "github.com/atlassian/smith/pkg/client/clientset_generated/clientset/typed/smith/v1"
	rest "k8s.io/client-go/rest"
	testing "k8s.io/client-go/testing"
)

type FakeSmithV1 struct {
	*testing.Fake
}

func (c *FakeSmithV1) Bundles(namespace string) v1.BundleInterface {
	return &FakeBundles{c, namespace}
}

// RESTClient returns a RESTClient that is used to communicate
// with API server by this client implementation.
func (c *FakeSmithV1) RESTClient() rest.Interface {
	var ret *rest.RESTClient
	return ret
}
