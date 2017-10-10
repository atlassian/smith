// Generated file, do not modify manually!
package fake

import (
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	schema "k8s.io/apimachinery/pkg/runtime/schema"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	testing "k8s.io/client-go/testing"
)

// FakeBundles implements BundleInterface
type FakeBundles struct {
	Fake *FakeSmithV1
	ns   string
}

var bundlesResource = schema.GroupVersionResource{Group: "smith.atlassian.com", Version: "v1", Resource: "bundles"}

var bundlesKind = schema.GroupVersionKind{Group: "smith.atlassian.com", Version: "v1", Kind: "Bundle"}

// Get takes name of the bundle, and returns the corresponding bundle object, and an error if there is any.
func (c *FakeBundles) Get(name string, options v1.GetOptions) (result *smith_v1.Bundle, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewGetAction(bundlesResource, c.ns, name), &smith_v1.Bundle{})

	if obj == nil {
		return nil, err
	}
	return obj.(*smith_v1.Bundle), err
}

// List takes label and field selectors, and returns the list of Bundles that match those selectors.
func (c *FakeBundles) List(opts v1.ListOptions) (result *smith_v1.BundleList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewListAction(bundlesResource, bundlesKind, c.ns, opts), &smith_v1.BundleList{})

	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &smith_v1.BundleList{}
	for _, item := range obj.(*smith_v1.BundleList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested bundles.
func (c *FakeBundles) Watch(opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewWatchAction(bundlesResource, c.ns, opts))

}

// Create takes the representation of a bundle and creates it.  Returns the server's representation of the bundle, and an error, if there is any.
func (c *FakeBundles) Create(bundle *smith_v1.Bundle) (result *smith_v1.Bundle, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewCreateAction(bundlesResource, c.ns, bundle), &smith_v1.Bundle{})

	if obj == nil {
		return nil, err
	}
	return obj.(*smith_v1.Bundle), err
}

// Update takes the representation of a bundle and updates it. Returns the server's representation of the bundle, and an error, if there is any.
func (c *FakeBundles) Update(bundle *smith_v1.Bundle) (result *smith_v1.Bundle, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateAction(bundlesResource, c.ns, bundle), &smith_v1.Bundle{})

	if obj == nil {
		return nil, err
	}
	return obj.(*smith_v1.Bundle), err
}

// Delete takes name of the bundle and deletes it. Returns an error if one occurs.
func (c *FakeBundles) Delete(name string, options *v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewDeleteAction(bundlesResource, c.ns, name), &smith_v1.Bundle{})

	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeBundles) DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error {
	action := testing.NewDeleteCollectionAction(bundlesResource, c.ns, listOptions)

	_, err := c.Fake.Invokes(action, &smith_v1.BundleList{})
	return err
}

// Patch applies the patch and returns the patched bundle.
func (c *FakeBundles) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *smith_v1.Bundle, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewPatchSubresourceAction(bundlesResource, c.ns, name, data, subresources...), &smith_v1.Bundle{})

	if obj == nil {
		return nil, err
	}
	return obj.(*smith_v1.Bundle), err
}
