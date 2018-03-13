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

// FakeTemplateRenders implements TemplateRenderInterface
type FakeTemplateRenders struct {
	Fake *FakeSmithV1
	ns   string
}

var templaterendersResource = schema.GroupVersionResource{Group: "smith.atlassian.com", Version: "v1", Resource: "templaterenders"}

var templaterendersKind = schema.GroupVersionKind{Group: "smith.atlassian.com", Version: "v1", Kind: "TemplateRender"}

// Get takes name of the templateRender, and returns the corresponding templateRender object, and an error if there is any.
func (c *FakeTemplateRenders) Get(name string, options v1.GetOptions) (result *smith_v1.TemplateRender, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewGetAction(templaterendersResource, c.ns, name), &smith_v1.TemplateRender{})

	if obj == nil {
		return nil, err
	}
	return obj.(*smith_v1.TemplateRender), err
}

// List takes label and field selectors, and returns the list of TemplateRenders that match those selectors.
func (c *FakeTemplateRenders) List(opts v1.ListOptions) (result *smith_v1.TemplateRenderList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewListAction(templaterendersResource, templaterendersKind, c.ns, opts), &smith_v1.TemplateRenderList{})

	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &smith_v1.TemplateRenderList{}
	for _, item := range obj.(*smith_v1.TemplateRenderList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested templateRenders.
func (c *FakeTemplateRenders) Watch(opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewWatchAction(templaterendersResource, c.ns, opts))

}

// Create takes the representation of a templateRender and creates it.  Returns the server's representation of the templateRender, and an error, if there is any.
func (c *FakeTemplateRenders) Create(templateRender *smith_v1.TemplateRender) (result *smith_v1.TemplateRender, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewCreateAction(templaterendersResource, c.ns, templateRender), &smith_v1.TemplateRender{})

	if obj == nil {
		return nil, err
	}
	return obj.(*smith_v1.TemplateRender), err
}

// Update takes the representation of a templateRender and updates it. Returns the server's representation of the templateRender, and an error, if there is any.
func (c *FakeTemplateRenders) Update(templateRender *smith_v1.TemplateRender) (result *smith_v1.TemplateRender, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateAction(templaterendersResource, c.ns, templateRender), &smith_v1.TemplateRender{})

	if obj == nil {
		return nil, err
	}
	return obj.(*smith_v1.TemplateRender), err
}

// Delete takes name of the templateRender and deletes it. Returns an error if one occurs.
func (c *FakeTemplateRenders) Delete(name string, options *v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewDeleteAction(templaterendersResource, c.ns, name), &smith_v1.TemplateRender{})

	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeTemplateRenders) DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error {
	action := testing.NewDeleteCollectionAction(templaterendersResource, c.ns, listOptions)

	_, err := c.Fake.Invokes(action, &smith_v1.TemplateRenderList{})
	return err
}

// Patch applies the patch and returns the patched templateRender.
func (c *FakeTemplateRenders) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *smith_v1.TemplateRender, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewPatchSubresourceAction(templaterendersResource, c.ns, name, data, subresources...), &smith_v1.TemplateRender{})

	if obj == nil {
		return nil, err
	}
	return obj.(*smith_v1.TemplateRender), err
}
