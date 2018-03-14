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

// FakeTemplates implements TemplateInterface
type FakeTemplates struct {
	Fake *FakeSmithV1
	ns   string
}

var templatesResource = schema.GroupVersionResource{Group: "smith.atlassian.com", Version: "v1", Resource: "templates"}

var templatesKind = schema.GroupVersionKind{Group: "smith.atlassian.com", Version: "v1", Kind: "Template"}

// Get takes name of the template, and returns the corresponding template object, and an error if there is any.
func (c *FakeTemplates) Get(name string, options v1.GetOptions) (result *smith_v1.Template, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewGetAction(templatesResource, c.ns, name), &smith_v1.Template{})

	if obj == nil {
		return nil, err
	}
	return obj.(*smith_v1.Template), err
}

// List takes label and field selectors, and returns the list of Templates that match those selectors.
func (c *FakeTemplates) List(opts v1.ListOptions) (result *smith_v1.TemplateList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewListAction(templatesResource, templatesKind, c.ns, opts), &smith_v1.TemplateList{})

	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &smith_v1.TemplateList{}
	for _, item := range obj.(*smith_v1.TemplateList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested templates.
func (c *FakeTemplates) Watch(opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewWatchAction(templatesResource, c.ns, opts))

}

// Create takes the representation of a template and creates it.  Returns the server's representation of the template, and an error, if there is any.
func (c *FakeTemplates) Create(template *smith_v1.Template) (result *smith_v1.Template, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewCreateAction(templatesResource, c.ns, template), &smith_v1.Template{})

	if obj == nil {
		return nil, err
	}
	return obj.(*smith_v1.Template), err
}

// Update takes the representation of a template and updates it. Returns the server's representation of the template, and an error, if there is any.
func (c *FakeTemplates) Update(template *smith_v1.Template) (result *smith_v1.Template, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateAction(templatesResource, c.ns, template), &smith_v1.Template{})

	if obj == nil {
		return nil, err
	}
	return obj.(*smith_v1.Template), err
}

// Delete takes name of the template and deletes it. Returns an error if one occurs.
func (c *FakeTemplates) Delete(name string, options *v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewDeleteAction(templatesResource, c.ns, name), &smith_v1.Template{})

	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeTemplates) DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error {
	action := testing.NewDeleteCollectionAction(templatesResource, c.ns, listOptions)

	_, err := c.Fake.Invokes(action, &smith_v1.TemplateList{})
	return err
}

// Patch applies the patch and returns the patched template.
func (c *FakeTemplates) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *smith_v1.Template, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewPatchSubresourceAction(templatesResource, c.ns, name, data, subresources...), &smith_v1.Template{})

	if obj == nil {
		return nil, err
	}
	return obj.(*smith_v1.Template), err
}
