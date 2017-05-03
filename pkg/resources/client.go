package resources

import (
	"errors"
	"fmt"
	"net"
	"os"

	"github.com/atlassian/smith"

	sc_v0 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog"
	scv1alpha1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1alpha1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/dynamic"
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

func ClientForResource(obj runtime.Object, coreDynamic, scDynamic dynamic.ClientPool, namespace string) (*dynamic.ResourceClient, error) {
	gvk := obj.GetObjectKind().GroupVersionKind()
	var clients dynamic.ClientPool
	if gvk.Group == sc_v0.GroupName {
		if scDynamic == nil {
			return nil, fmt.Errorf("client for Service Catalog is not configured, cannot work with object %s", gvk)
		}
		clients = scDynamic
	} else {
		clients = coreDynamic
	}
	client, err := clients.ClientForGroupVersionKind(gvk)
	if err != nil {
		return nil, err
	}

	plural, _ := meta.KindToResource(gvk)

	return client.Resource(&metav1.APIResource{
		Name:       plural.Resource,
		Namespaced: true,
		Kind:       gvk.Kind,
	}, namespace), nil
}
