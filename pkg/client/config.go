package client

import (
	"net"
	"os"

	"github.com/pkg/errors"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

func ConfigFromEnv() (*rest.Config, error) {
	host, port := os.Getenv("KUBERNETES_SERVICE_HOST"), os.Getenv("KUBERNETES_SERVICE_PORT")
	if host == "" || port == "" {
		return nil, errors.New("unable to load cluster configuration, KUBERNETES_SERVICE_HOST and KUBERNETES_SERVICE_PORT must be defined")
	}
	CAFile, CertFile, KeyFile := os.Getenv("KUBERNETES_CA_PATH"), os.Getenv("KUBERNETES_CLIENT_CERT"), os.Getenv("KUBERNETES_CLIENT_KEY")
	if CAFile == "" || CertFile == "" || KeyFile == "" {
		return nil, errors.New("unable to load TLS configuration, KUBERNETES_CA_PATH, KUBERNETES_CLIENT_CERT and KUBERNETES_CLIENT_KEY must be defined")
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

func LoadConfig(configFileFrom, configFileName, configContext string) (*rest.Config, error) {
	var config *rest.Config
	var err error

	switch configFileFrom {
	case "in-cluster":
		config, err = rest.InClusterConfig()
	case "environment":
		config, err = ConfigFromEnv()
	case "file":
		var configAPI *clientcmdapi.Config
		configAPI, err = clientcmd.LoadFromFile(configFileName)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to load REST client configuration from file %q", configFileName)
		}
		config, err = clientcmd.NewDefaultClientConfig(*configAPI, &clientcmd.ConfigOverrides{
			CurrentContext: configContext,
		}).ClientConfig()
	default:
		err = errors.New("invalid value for 'client config from' parameter")
	}
	if err != nil {
		return nil, errors.Wrapf(err, "failed to load REST client configuration from %q", configFileFrom)
	}
	return config, nil
}
