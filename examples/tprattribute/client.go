package tprattribute

import (
	"github.com/atlassian/smith"

	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	ext_v1b1 "k8s.io/client-go/pkg/apis/extensions/v1beta1"
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

func SleeperTpr() *ext_v1b1.ThirdPartyResource {
	return &ext_v1b1.ThirdPartyResource{
		ObjectMeta: meta_v1.ObjectMeta{
			Name: SleeperResourceName,
			Annotations: map[string]string{
				smith.TprFieldPathAnnotation:  SleeperReadyStatePath,
				smith.TprFieldValueAnnotation: string(SleeperReadyStateValue),
			},
		},
		Description: "Sleeper TPR example",
		Versions: []ext_v1b1.APIVersion{
			{Name: SleeperResourceVersion},
		},
	}
}
