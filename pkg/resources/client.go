package resources

import (
	"errors"
	"net"
	"os"

	"github.com/atlassian/smith"

	scv1alpha1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	apiv1 "k8s.io/client-go/pkg/api/v1"
	appsv1beta1 "k8s.io/client-go/pkg/apis/apps/v1beta1"
	extensions "k8s.io/client-go/pkg/apis/extensions/v1beta1"
	settings "k8s.io/client-go/pkg/apis/settings/v1alpha1"
	"k8s.io/client-go/rest"
)

func BundleScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	smith.AddToScheme(scheme)
	scheme.AddUnversionedTypes(apiv1.SchemeGroupVersion, &metav1.Status{})
	return scheme
}

func FullScheme(serviceCatalog bool) (*runtime.Scheme, error) {
	scheme := runtime.NewScheme()
	smith.AddToScheme(scheme)
	var sb runtime.SchemeBuilder
	sb.Register(extensions.SchemeBuilder...)
	sb.Register(apiv1.SchemeBuilder...)
	sb.Register(appsv1beta1.SchemeBuilder...)
	sb.Register(settings.SchemeBuilder...)
	if serviceCatalog {
		sb.Register(scv1alpha1.SchemeBuilder...)
	} else {
		scheme.AddUnversionedTypes(apiv1.SchemeGroupVersion, &metav1.Status{})
	}
	if err := sb.AddToScheme(scheme); err != nil {
		return nil, err
	}
	return scheme, nil
}

func GetBundleTprClient(cfg *rest.Config, scheme *runtime.Scheme) (*rest.RESTClient, error) {
	groupVersion := schema.GroupVersion{
		Group:   smith.SmithResourceGroup,
		Version: smith.BundleResourceVersion,
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

func ConfigFromEnv() (*rest.Config, error) {
	host, port := os.Getenv("KUBERNETES_SERVICE_HOST"), os.Getenv("KUBERNETES_SERVICE_PORT")
	if host == "" || port == "" {
		return nil, errors.New("Unable to load cluster configuration, KUBERNETES_SERVICE_HOST and KUBERNETES_SERVICE_PORT must be defined")
	}
	CAFile, CertFile, KeyFile := os.Getenv("KUBERNETES_CA_PATH"), os.Getenv("KUBERNETES_CLIENT_CERT"), os.Getenv("KUBERNETES_CLIENT_KEY")
	if CAFile == "" || CertFile == "" || KeyFile == "" {
		return nil, errors.New("Unable to load TLS configuration, KUBERNETES_CA_PATH, KUBERNETES_CLIENT_CERT and KUBERNETES_CLIENT_KEY must be defined")
	}
	return &rest.Config{
		Host: "https://" + net.JoinHostPort(host, port),
		TLSClientConfig: rest.TLSClientConfig{
			CAFile:   CAFile,
			CertFile: CertFile,
			KeyFile:  KeyFile,
		},
	}, nil
}
