package sleeper

import (
	"github.com/atlassian/smith"
	sleeper_v1 "github.com/atlassian/smith/examples/sleeper/pkg/apis/sleeper/v1"

	core_v1 "k8s.io/api/core/v1"
	apiext_v1b1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/rest"
)

func Scheme() (*runtime.Scheme, error) {
	scheme := runtime.NewScheme()
	scheme.AddUnversionedTypes(core_v1.SchemeGroupVersion, &meta_v1.Status{})
	if err := sleeper_v1.AddToScheme(scheme); err != nil {
		return nil, err
	}
	return scheme, nil
}

func Client(cfg *rest.Config) (*rest.RESTClient, error) {
	scheme, err := Scheme()
	if err != nil {
		return nil, err
	}
	config := *cfg
	config.GroupVersion = &sleeper_v1.SchemeGroupVersion
	config.APIPath = "/apis"
	config.ContentType = runtime.ContentTypeJSON
	config.NegotiatedSerializer = serializer.DirectCodecFactory{CodecFactory: serializer.NewCodecFactory(scheme)}

	return rest.RESTClientFor(&config)
}

func Crd() *apiext_v1b1.CustomResourceDefinition {
	return &apiext_v1b1.CustomResourceDefinition{
		ObjectMeta: meta_v1.ObjectMeta{
			Name: sleeper_v1.SleeperResourceName,
			Annotations: map[string]string{
				smith.CrFieldPathAnnotation:  sleeper_v1.SleeperReadyStatePath,
				smith.CrFieldValueAnnotation: string(sleeper_v1.SleeperReadyStateValue),
				smith.CrdSupportEnabled:      "true",
			},
		},
		Spec: apiext_v1b1.CustomResourceDefinitionSpec{
			Group:   sleeper_v1.GroupName,
			Version: sleeper_v1.SleeperResourceVersion,
			Names: apiext_v1b1.CustomResourceDefinitionNames{
				Plural:   sleeper_v1.SleeperResourcePlural,
				Singular: sleeper_v1.SleeperResourceSingular,
				Kind:     sleeper_v1.SleeperResourceKind,
			},
		},
	}
}
