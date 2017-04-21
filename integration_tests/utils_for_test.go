// +build integration

package integration_tests

import (
	"testing"

	"github.com/atlassian/smith"
	"github.com/atlassian/smith/examples/tprattribute"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	apiv1 "k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

const (
	useNamespace = apiv1.NamespaceDefault
)

func assertCondition(t *testing.T, bundle *smith.Bundle, conditionType smith.BundleConditionType, status apiv1.ConditionStatus) {
	_, condition := bundle.GetCondition(conditionType)
	if assert.NotNil(t, condition) {
		assert.Equal(t, status, condition.Status)
	}
}

func smithScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	scheme.AddUnversionedTypes(apiv1.SchemeGroupVersion, &metav1.Status{})
	smith.AddToScheme(scheme)
	return scheme
}

func sleeperScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	scheme.AddUnversionedTypes(apiv1.SchemeGroupVersion, &metav1.Status{})
	tprattribute.AddToScheme(scheme)
	return scheme
}

func bundleInformer(bundleClient cache.Getter) cache.SharedIndexInformer {
	return cache.NewSharedIndexInformer(
		cache.NewListWatchFromClient(bundleClient, smith.BundleResourcePath, metav1.NamespaceAll, fields.Everything()),
		&smith.Bundle{},
		0,
		cache.Indexers{})
}

func cleanupBundle(t *testing.T, bundleClient *rest.RESTClient, clients dynamic.ClientPool, bundleCreated *bool, bundle *smith.Bundle, bundleNamespace string) {
	if !*bundleCreated {
		return
	}
	t.Logf("Deleting bundle %s", bundle.Metadata.Name)
	assert.NoError(t, bundleClient.Delete().
		Namespace(bundleNamespace).
		Resource(smith.BundleResourcePath).
		Name(bundle.Metadata.Name).
		Do().
		Error())
	for _, resource := range bundle.Spec.Resources {
		t.Logf("Deleting resource %s", resource.Spec.GetName())
		gv, err := schema.ParseGroupVersion(resource.Spec.GetAPIVersion())
		if !assert.NoError(t, err) {
			continue
		}
		client, err := clients.ClientForGroupVersionKind(gv.WithKind(resource.Spec.GetKind()))
		if !assert.NoError(t, err) {
			continue
		}
		plural, _ := meta.KindToResource(resource.Spec.GroupVersionKind())
		assert.NoError(t, client.Resource(&metav1.APIResource{
			Name:       plural.Resource,
			Namespaced: true,
			Kind:       resource.Spec.GetKind(),
		}, bundleNamespace).Delete(resource.Spec.GetName(), nil))
	}
}

func awaitBundleReady(obj runtime.Object) bool {
	b := obj.(*smith.Bundle)
	_, cond := b.GetCondition(smith.BundleReady)
	return cond != nil && cond.Status == apiv1.ConditionTrue
}
