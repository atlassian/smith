package tprattribute

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/rest"
)

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
