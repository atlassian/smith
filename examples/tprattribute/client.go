package tprattribute

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/pkg/api"
	"k8s.io/client-go/rest"
)

func GetSleeperScheme() *runtime.Scheme {
	groupVersion := schema.GroupVersion{
		Group:   SleeperResourceGroup,
		Version: SleeperResourceVersion,
	}

	schemeBuilder := runtime.NewSchemeBuilder(func(scheme *runtime.Scheme) error {
		scheme.AddKnownTypes(groupVersion,
			&Sleeper{},
			&SleeperList{},
		)
		scheme.AddUnversionedTypes(api.Unversioned, &metav1.Status{})
		metav1.AddToGroupVersion(scheme, groupVersion)
		return nil
	})

	scheme := runtime.NewScheme()
	if err := schemeBuilder.AddToScheme(scheme); err != nil {
		panic(err)
	}
	return scheme
}

func GetSleeperTprClient(cfg *rest.Config, scheme *runtime.Scheme) (*rest.RESTClient, error) {
	groupVersion := schema.GroupVersion{
		Group:   SleeperResourceGroup,
		Version: SleeperResourceVersion,
	}

	config := *cfg
	config.GroupVersion = &groupVersion
	config.APIPath = "/apis"
	config.ContentType = runtime.ContentTypeJSON
	config.NegotiatedSerializer = serializer.DirectCodecFactory{CodecFactory: serializer.NewCodecFactory(scheme)}

	client, err := rest.RESTClientFor(&config)

	if err != nil {
		return nil, err
	}

	return client, nil
}
