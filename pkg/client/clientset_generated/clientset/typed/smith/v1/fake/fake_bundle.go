// Generated file, do not modify manually!
package fake

import (
	v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

func (c *FakeBundles) Create(bundle *v1.Bundle) (result *v1.Bundle, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewCreateAction(bundlesResource, c.ns, bundle), &v1.Bundle{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1.Bundle), err
}

func (c *FakeBundles) Update(bundle *v1.Bundle) (result *v1.Bundle, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateAction(bundlesResource, c.ns, bundle), &v1.Bundle{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1.Bundle), err
}

func (c *FakeBundles) Delete(name string, options *meta_v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewDeleteAction(bundlesResource, c.ns, name), &v1.Bundle{})

	return err
}

func (c *FakeBundles) DeleteCollection(options *meta_v1.DeleteOptions, listOptions meta_v1.ListOptions) error {
	action := testing.NewDeleteCollectionAction(bundlesResource, c.ns, listOptions)

	_, err := c.Fake.Invokes(action, &v1.BundleList{})
	return err
}

func (c *FakeBundles) Get(name string, options meta_v1.GetOptions) (result *v1.Bundle, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewGetAction(bundlesResource, c.ns, name), &v1.Bundle{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1.Bundle), err
}

func (c *FakeBundles) List(opts meta_v1.ListOptions) (result *v1.BundleList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewListAction(bundlesResource, bundlesKind, c.ns, opts), &v1.BundleList{})

	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &v1.BundleList{}
	for _, item := range obj.(*v1.BundleList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested bundles.
func (c *FakeBundles) Watch(opts meta_v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewWatchAction(bundlesResource, c.ns, opts))

}

// Patch applies the patch and returns the patched bundle.
func (c *FakeBundles) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1.Bundle, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewPatchSubresourceAction(bundlesResource, c.ns, name, data, subresources...), &v1.Bundle{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1.Bundle), err
}
