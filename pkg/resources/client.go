package resources

import (
	"github.com/atlassian/smith"

	"k8s.io/client-go/pkg/api"
	apiv1 "k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/pkg/runtime"
	"k8s.io/client-go/pkg/runtime/schema"
	"k8s.io/client-go/pkg/runtime/serializer"
	"k8s.io/client-go/rest"
)

func GetTemplateTprClient(cfg *rest.Config) (*rest.RESTClient, *runtime.Scheme, error) {
	groupVersion := schema.GroupVersion{
		Group:   smith.SmithResourceGroup,
		Version: smith.TemplateResourceVersion,
	}

	schemeBuilder := runtime.NewSchemeBuilder(func(scheme *runtime.Scheme) error {
		scheme.AddKnownTypes(
			groupVersion,
			&smith.Template{},
			&smith.TemplateList{},
			&apiv1.ListOptions{},
			&apiv1.DeleteOptions{},
		)
		return scheme.AddConversionFuncs(
			api.Convert_v1_TypeMeta_To_v1_TypeMeta,

			api.Convert_unversioned_ListMeta_To_unversioned_ListMeta,

			api.Convert_intstr_IntOrString_To_intstr_IntOrString,

			api.Convert_unversioned_Time_To_unversioned_Time,

			api.Convert_Slice_string_To_unversioned_Time,

			api.Convert_resource_Quantity_To_resource_Quantity,

			api.Convert_string_To_labels_Selector,
			api.Convert_labels_Selector_To_string,

			api.Convert_string_To_fields_Selector,
			api.Convert_fields_Selector_To_string,

			api.Convert_Pointer_bool_To_bool,
			api.Convert_bool_To_Pointer_bool,

			api.Convert_Pointer_string_To_string,
			api.Convert_string_To_Pointer_string,

			api.Convert_Pointer_int64_To_int,
			api.Convert_int_To_Pointer_int64,

			api.Convert_Pointer_int32_To_int32,
			api.Convert_int32_To_Pointer_int32,

			api.Convert_Pointer_float64_To_float64,
			api.Convert_float64_To_Pointer_float64,

			api.Convert_map_to_unversioned_LabelSelector,
			api.Convert_unversioned_LabelSelector_to_map,
		)
	})

	scheme := runtime.NewScheme()
	if err := schemeBuilder.AddToScheme(scheme); err != nil {
		return nil, nil, err
	}

	config := *cfg
	config.GroupVersion = &groupVersion
	config.APIPath = "/apis"
	config.ContentType = runtime.ContentTypeJSON
	config.NegotiatedSerializer = serializer.DirectCodecFactory{CodecFactory: serializer.NewCodecFactory(scheme)}

	client, err := rest.RESTClientFor(&config)

	return client, scheme, err
}
