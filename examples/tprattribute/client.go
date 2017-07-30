package tprattribute

import (
	"github.com/atlassian/smith"

	apiext_v1b1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/rest"
)

func GetSleeperClient(cfg *rest.Config, scheme *runtime.Scheme) (*rest.RESTClient, error) {
	groupVersion := schema.GroupVersion{
		Group:   SleeperResourceGroup,
		Version: SleeperResourceVersion,
	}

	config := *cfg
	config.GroupVersion = &groupVersion
	config.APIPath = "/apis"
	config.ContentType = runtime.ContentTypeJSON
	config.NegotiatedSerializer = serializer.DirectCodecFactory{CodecFactory: serializer.NewCodecFactory(scheme)}

	return rest.RESTClientFor(&config)
}

func SleeperCrd() *apiext_v1b1.CustomResourceDefinition {
	return &apiext_v1b1.CustomResourceDefinition{
		ObjectMeta: meta_v1.ObjectMeta{
			Name: SleeperResourceName,
			Annotations: map[string]string{
				smith.CrFieldPathAnnotation:  SleeperReadyStatePath,
				smith.CrFieldValueAnnotation: string(SleeperReadyStateValue),
			},
		},
		Spec: apiext_v1b1.CustomResourceDefinitionSpec{
			Group:   SleeperResourceGroup,
			Version: SleeperResourceVersion,
			Names: apiext_v1b1.CustomResourceDefinitionNames{
				Plural:   SleeperResourcePlural,
				Singular: SleeperResourceSingular,
				Kind:     SleeperResourceKind,
			},
		},
	}
}
