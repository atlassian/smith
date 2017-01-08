package resources

import (
	"github.com/atlassian/smith"

	"k8s.io/client-go/pkg/api/unversioned"
	apiv1 "k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/pkg/runtime"
	"k8s.io/client-go/pkg/runtime/serializer"
	"k8s.io/client-go/rest"
)

func GetTemplateTprClient(cfg *rest.Config) (*rest.RESTClient, error) {
	config := *cfg

	groupVersion := unversioned.GroupVersion{
		Group:   smith.SmithResourceGroup,
		Version: smith.TemplateResourceVersion,
	}

	scheme := runtime.NewScheme()
	scheme.AddKnownTypes(
		groupVersion,
		&smith.Template{},
		&smith.TemplateList{},
		&apiv1.ListOptions{},
		&apiv1.DeleteOptions{},
	)

	config.GroupVersion = &groupVersion
	config.APIPath = "/apis"
	config.ContentType = runtime.ContentTypeJSON
	config.NegotiatedSerializer = serializer.DirectCodecFactory{CodecFactory: serializer.NewCodecFactory(scheme)}

	return rest.RESTClientFor(&config)
}
