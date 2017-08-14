// Generated file, do not modify manually!
package fake

import (
	clientset "github.com/atlassian/smith/pkg/client/clientset_generated/clientset"
	smithv1 "github.com/atlassian/smith/pkg/client/clientset_generated/clientset/typed/smith/v1"
	fakesmithv1 "github.com/atlassian/smith/pkg/client/clientset_generated/clientset/typed/smith/v1/fake"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/discovery"
	fakediscovery "k8s.io/client-go/discovery/fake"
	"k8s.io/client-go/testing"
)

// NewSimpleClientset returns a clientset that will respond with the provided objects.
// It's backed by a very simple object tracker that processes creates, updates and deletions as-is,
// without applying any validations and/or defaults. It shouldn't be considered a replacement
// for a real clientset and is mostly useful in simple unit tests.
func NewSimpleClientset(objects ...runtime.Object) *Clientset {
	o := testing.NewObjectTracker(scheme, codecs.UniversalDecoder())
	for _, obj := range objects {
		if err := o.Add(obj); err != nil {
			panic(err)
		}
	}

	fakePtr := testing.Fake{}
	fakePtr.AddReactor("*", "*", testing.ObjectReaction(o))

	fakePtr.AddWatchReactor("*", testing.DefaultWatchReactor(watch.NewFake(), nil))

	return &Clientset{fakePtr}
}

// Clientset implements clientset.Interface. Meant to be embedded into a
// struct to get a default implementation. This makes faking out just the method
// you want to test easier.
type Clientset struct {
	testing.Fake
}

func (c *Clientset) Discovery() discovery.DiscoveryInterface {
	return &fakediscovery.FakeDiscovery{Fake: &c.Fake}
}

var _ clientset.Interface = &Clientset{}

// SmithV1 retrieves the SmithV1Client
func (c *Clientset) SmithV1() smithv1.SmithV1Interface {
	return &fakesmithv1.FakeSmithV1{Fake: &c.Fake}
}

// Smith retrieves the SmithV1Client
func (c *Clientset) Smith() smithv1.SmithV1Interface {
	return &fakesmithv1.FakeSmithV1{Fake: &c.Fake}
}
