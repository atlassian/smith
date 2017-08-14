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

// BundlesGetter has a method to return a BundleInterface.
// A group's client should implement this interface.
type BundlesGetter interface {
	Bundles(namespace string) BundleInterface
}

// BundleInterface has methods to work with Bundle resources.
type BundleInterface interface {
	Create(*v1.Bundle) (*v1.Bundle, error)
	Update(*v1.Bundle) (*v1.Bundle, error)
	Delete(name string, options *meta_v1.DeleteOptions) error
	DeleteCollection(options *meta_v1.DeleteOptions, listOptions meta_v1.ListOptions) error
	Get(name string, options meta_v1.GetOptions) (*v1.Bundle, error)
	List(opts meta_v1.ListOptions) (*v1.BundleList, error)
	Watch(opts meta_v1.ListOptions) (watch.Interface, error)
	Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1.Bundle, err error)
	BundleExpansion
}

// bundles implements BundleInterface
type bundles struct {
	client rest.Interface
	ns     string
}

// newBundles returns a Bundles
func newBundles(c *SmithV1Client, namespace string) *bundles {
	return &bundles{
		client: c.RESTClient(),
		ns:     namespace,
	}
}

// Create takes the representation of a bundle and creates it.  Returns the server's representation of the bundle, and an error, if there is any.
func (c *bundles) Create(bundle *v1.Bundle) (result *v1.Bundle, err error) {
	result = &v1.Bundle{}
	err = c.client.Post().
		Namespace(c.ns).
		Resource("bundles").
		Body(bundle).
		Do().
		Into(result)
	return
}

// Update takes the representation of a bundle and updates it. Returns the server's representation of the bundle, and an error, if there is any.
func (c *bundles) Update(bundle *v1.Bundle) (result *v1.Bundle, err error) {
	result = &v1.Bundle{}
	err = c.client.Put().
		Namespace(c.ns).
		Resource("bundles").
		Name(bundle.Name).
		Body(bundle).
		Do().
		Into(result)
	return
}

// Delete takes name of the bundle and deletes it. Returns an error if one occurs.
func (c *bundles) Delete(name string, options *meta_v1.DeleteOptions) error {
	return c.client.Delete().
		Namespace(c.ns).
		Resource("bundles").
		Name(name).
		Body(options).
		Do().
		Error()
}

// DeleteCollection deletes a collection of objects.
func (c *bundles) DeleteCollection(options *meta_v1.DeleteOptions, listOptions meta_v1.ListOptions) error {
	return c.client.Delete().
		Namespace(c.ns).
		Resource("bundles").
		VersionedParams(&listOptions, scheme.ParameterCodec).
		Body(options).
		Do().
		Error()
}

// Get takes name of the bundle, and returns the corresponding bundle object, and an error if there is any.
func (c *bundles) Get(name string, options meta_v1.GetOptions) (result *v1.Bundle, err error) {
	result = &v1.Bundle{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("bundles").
		Name(name).
		VersionedParams(&options, scheme.ParameterCodec).
		Do().
		Into(result)
	return
}

// List takes label and field selectors, and returns the list of Bundles that match those selectors.
func (c *bundles) List(opts meta_v1.ListOptions) (result *v1.BundleList, err error) {
	result = &v1.BundleList{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("bundles").
		VersionedParams(&opts, scheme.ParameterCodec).
		Do().
		Into(result)
	return
}

// Watch returns a watch.Interface that watches the requested bundles.
func (c *bundles) Watch(opts meta_v1.ListOptions) (watch.Interface, error) {
	opts.Watch = true
	return c.client.Get().
		Namespace(c.ns).
		Resource("bundles").
		VersionedParams(&opts, scheme.ParameterCodec).
		Watch()
}

// Patch applies the patch and returns the patched bundle.
func (c *bundles) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1.Bundle, err error) {
	result = &v1.Bundle{}
	err = c.client.Patch(pt).
		Namespace(c.ns).
		Resource("bundles").
		SubResource(subresources...).
		Name(name).
		Body(data).
		Do().
		Into(result)
	return
}
