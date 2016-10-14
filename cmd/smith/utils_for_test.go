package main

import (
	"crypto/tls"
	"crypto/x509"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/atlassian/smith"
	"github.com/atlassian/smith/pkg/client"

	"github.com/stretchr/testify/require"
)

func newClient(t *testing.T, r *require.Assertions) *client.ResourceClient {
	host, port := os.Getenv("KUBERNETES_SERVICE_HOST"), os.Getenv("KUBERNETES_SERVICE_PORT")
	if len(host) == 0 || len(port) == 0 {
		t.Fatal("Unable to load cluster configuration, KUBERNETES_SERVICE_HOST and KUBERNETES_SERVICE_PORT must be defined")
	}
	var certPool *x509.CertPool
	rootCA := os.Getenv("KUBERNETES_CA_PATH")
	if rootCA != "" {
		CAData, err := ioutil.ReadFile(rootCA)
		r.Nil(err)
		certPool = x509.NewCertPool()
		if !certPool.AppendCertsFromPEM(CAData) {
			t.Fatalf("Failed to add certificate from %s", rootCA)
		}
	}
	var clientCerts []tls.Certificate
	clientCert, clientKey := os.Getenv("KUBERNETES_CLIENT_CERT"), os.Getenv("KUBERNETES_CLIENT_KEY")
	if clientCert != "" && clientKey != "" {
		cert, err := tls.LoadX509KeyPair(clientCert, clientKey)
		r.Nil(err)
		clientCerts = []tls.Certificate{cert}
	}
	return &client.ResourceClient{
		Scheme:   "https",
		HostPort: net.JoinHostPort(host, port),
		Client: http.Client{
			Timeout: 10 * time.Minute,
			Transport: &http.Transport{
				Proxy:               http.ProxyFromEnvironment,
				TLSHandshakeTimeout: 10 * time.Second,
				TLSClientConfig: &tls.Config{
					Certificates: clientCerts,
					MinVersion:   tls.VersionTLS12,
					RootCAs:      certPool,
					//InsecureSkipVerify: true,
				},
				DialContext: (&net.Dialer{
					Timeout:   30 * time.Second,
					KeepAlive: 30 * time.Second,
				}).DialContext,
			},
		},
	}
}

func genericEventFactory() interface{} {
	return &smith.GenericWatchEvent{}
}
