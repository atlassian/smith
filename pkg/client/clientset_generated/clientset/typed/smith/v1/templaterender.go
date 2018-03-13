// Generated file, do not modify manually!
package v1

import (
	v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	scheme "github.com/atlassian/smith/pkg/client/clientset_generated/clientset/scheme"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	rest "k8s.io/client-go/rest"
)

// TemplateRendersGetter has a method to return a TemplateRenderInterface.
// A group's client should implement this interface.
type TemplateRendersGetter interface {
	TemplateRenders(namespace string) TemplateRenderInterface
}

// TemplateRenderInterface has methods to work with TemplateRender resources.
type TemplateRenderInterface interface {
	Create(*v1.TemplateRender) (*v1.TemplateRender, error)
	Update(*v1.TemplateRender) (*v1.TemplateRender, error)
	Delete(name string, options *meta_v1.DeleteOptions) error
	DeleteCollection(options *meta_v1.DeleteOptions, listOptions meta_v1.ListOptions) error
	Get(name string, options meta_v1.GetOptions) (*v1.TemplateRender, error)
	List(opts meta_v1.ListOptions) (*v1.TemplateRenderList, error)
	Watch(opts meta_v1.ListOptions) (watch.Interface, error)
	Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1.TemplateRender, err error)
	TemplateRenderExpansion
}

// templateRenders implements TemplateRenderInterface
type templateRenders struct {
	client rest.Interface
	ns     string
}

// newTemplateRenders returns a TemplateRenders
func newTemplateRenders(c *SmithV1Client, namespace string) *templateRenders {
	return &templateRenders{
		client: c.RESTClient(),
		ns:     namespace,
	}
}

// Get takes name of the templateRender, and returns the corresponding templateRender object, and an error if there is any.
func (c *templateRenders) Get(name string, options meta_v1.GetOptions) (result *v1.TemplateRender, err error) {
	result = &v1.TemplateRender{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("templaterenders").
		Name(name).
		VersionedParams(&options, scheme.ParameterCodec).
		Do().
		Into(result)
	return
}

// List takes label and field selectors, and returns the list of TemplateRenders that match those selectors.
func (c *templateRenders) List(opts meta_v1.ListOptions) (result *v1.TemplateRenderList, err error) {
	result = &v1.TemplateRenderList{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("templaterenders").
		VersionedParams(&opts, scheme.ParameterCodec).
		Do().
		Into(result)
	return
}

// Watch returns a watch.Interface that watches the requested templateRenders.
func (c *templateRenders) Watch(opts meta_v1.ListOptions) (watch.Interface, error) {
	opts.Watch = true
	return c.client.Get().
		Namespace(c.ns).
		Resource("templaterenders").
		VersionedParams(&opts, scheme.ParameterCodec).
		Watch()
}

// Create takes the representation of a templateRender and creates it.  Returns the server's representation of the templateRender, and an error, if there is any.
func (c *templateRenders) Create(templateRender *v1.TemplateRender) (result *v1.TemplateRender, err error) {
	result = &v1.TemplateRender{}
	err = c.client.Post().
		Namespace(c.ns).
		Resource("templaterenders").
		Body(templateRender).
		Do().
		Into(result)
	return
}

// Update takes the representation of a templateRender and updates it. Returns the server's representation of the templateRender, and an error, if there is any.
func (c *templateRenders) Update(templateRender *v1.TemplateRender) (result *v1.TemplateRender, err error) {
	result = &v1.TemplateRender{}
	err = c.client.Put().
		Namespace(c.ns).
		Resource("templaterenders").
		Name(templateRender.Name).
		Body(templateRender).
		Do().
		Into(result)
	return
}

// Delete takes name of the templateRender and deletes it. Returns an error if one occurs.
func (c *templateRenders) Delete(name string, options *meta_v1.DeleteOptions) error {
	return c.client.Delete().
		Namespace(c.ns).
		Resource("templaterenders").
		Name(name).
		Body(options).
		Do().
		Error()
}

// DeleteCollection deletes a collection of objects.
func (c *templateRenders) DeleteCollection(options *meta_v1.DeleteOptions, listOptions meta_v1.ListOptions) error {
	return c.client.Delete().
		Namespace(c.ns).
		Resource("templaterenders").
		VersionedParams(&listOptions, scheme.ParameterCodec).
		Body(options).
		Do().
		Error()
}

// Patch applies the patch and returns the patched templateRender.
func (c *templateRenders) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1.TemplateRender, err error) {
	result = &v1.TemplateRender{}
	err = c.client.Patch(pt).
		Namespace(c.ns).
		Resource("templaterenders").
		SubResource(subresources...).
		Name(name).
		Body(data).
		Do().
		Into(result)
	return
}
