package client

import (
	"time"

	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"

	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	api_v1 "k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

func BundleScheme() (*runtime.Scheme, error) {
	scheme := runtime.NewScheme()
	if err := smith_v1.AddToScheme(scheme); err != nil {
		return nil, err
	}
	scheme.AddUnversionedTypes(api_v1.SchemeGroupVersion, &meta_v1.Status{})
	return scheme, nil
}

func BundleClient(cfg *rest.Config, scheme *runtime.Scheme) (*rest.RESTClient, error) {
	groupVersion := schema.GroupVersion{
		Group:   smith_v1.GroupName,
		Version: smith_v1.BundleResourceVersion,
	}

	config := *cfg
	config.GroupVersion = &groupVersion
	config.APIPath = "/apis"
	config.ContentType = runtime.ContentTypeJSON
	config.NegotiatedSerializer = serializer.DirectCodecFactory{CodecFactory: serializer.NewCodecFactory(scheme)}

	return rest.RESTClientFor(&config)
}

func BundleInformer(bundleClient cache.Getter, namespace string, resyncPeriod time.Duration) cache.SharedIndexInformer {
	return cache.NewSharedIndexInformer(
		cache.NewListWatchFromClient(bundleClient, smith_v1.BundleResourcePlural, namespace, fields.Everything()),
		&smith_v1.Bundle{},
		resyncPeriod,
		cache.Indexers{})
}
