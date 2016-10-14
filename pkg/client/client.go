package client

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/atlassian/smith"
)

const (
	// maxResponseSize is the maximum response size we are willing to read.
	maxResponseSize = 1 * 1024 * 1024
	// maxStatusResponseSize is the maximum status response size we are willing to read.
	maxStatusResponseSize = 10 * 1024
)

type StreamHandler func(io.Reader) error

type ResponseHandler func(*http.Response) error

// ResultFactory creates new instances of an object, that is used as JSON deserialization target.
// Must be safe for concurrent use.
type ResultFactory func() interface{}

type ResourceClient struct {
	Scheme      string
	HostPort    string
	BearerToken string
	Agent       string
	Client      http.Client
}

type StatusError struct {
	msg    string
	status smith.Status
}

func (se *StatusError) Error() string {
	return se.msg
}

func (se *StatusError) Status() smith.Status {
	return se.status
}

func (c *ResourceClient) Get(ctx context.Context, groupVersion, namespace, resource, name string, args url.Values, into interface{}) error {
	return c.Do(ctx, "GET", groupVersion, namespace, resource, name, "", args, http.StatusOK, nil, into)
}

func (c *ResourceClient) List(ctx context.Context, groupVersion, namespace, resource string, args url.Values, into interface{}) error {
	return c.Do(ctx, "GET", groupVersion, namespace, resource, "", "", args, http.StatusOK, nil, into)
}

func (c *ResourceClient) Create(ctx context.Context, groupVersion, namespace, resource string, request interface{}, response interface{}) error {
	return c.Do(ctx, "POST", groupVersion, namespace, resource, "", "", nil, http.StatusCreated, request, response)
}

func (c *ResourceClient) Update(ctx context.Context, groupVersion, namespace, resource string, request interface{}, response interface{}) error {
	return c.Do(ctx, "PUT", groupVersion, namespace, resource, "", "", nil, http.StatusOK, request, response)
}

func (c *ResourceClient) Delete(ctx context.Context, groupVersion, namespace, resource, name string) error {
	return c.Do(ctx, "DELETE", groupVersion, namespace, resource, name, "", nil, http.StatusOK, nil, nil)
}

func (c *ResourceClient) UpdateStatus(ctx context.Context, groupVersion, namespace, resource, name string, request interface{}, response interface{}) error {
	return c.Do(ctx, "PUT", groupVersion, namespace, resource, name, "status", nil, http.StatusOK, request, response)
}

func (c *ResourceClient) Watch(ctx context.Context, groupVersion, namespace, resource string, args url.Values, rf ResultFactory) <-chan interface{} {
	log.Printf("Watching gv=%s ns=%s res=%s with args %s", groupVersion, namespace, resource, args)
	type hasResourceVersion interface {
		ResourceVersion() string
	}

	results := make(chan interface{})
	a := url.Values{}
	if len(args) > 0 {
		for k, v := range args {
			a[k] = append(v[0:0], v...)
		}
	}
	args = a
	go func() {
		defer close(results)
		args.Set("watch", "true")
		for {
			var rv string
			err := c.DoCheckResponse(ctx, "GET", groupVersion, namespace, resource, "", "", args, http.StatusOK, nil, func(r io.Reader) error {
				decoder := json.NewDecoder(r)
				for {
					res := rf()
					if err := decoder.Decode(res); err != nil {
						return err
					}
					select {
					case <-ctx.Done():
						return ctx.Err()
					case results <- res:
					}
					if rvResource, ok := res.(hasResourceVersion); ok {
						rv = rvResource.ResourceVersion()
					}
				}
			})
			if err != nil {
				select {
				case <-ctx.Done():
					return
				default:
					select {
					case <-ctx.Done():
						return
					case results <- err:
					}
				}
			}
			// Delay after failed/closed connection
			select {
			case <-ctx.Done():
				//results <- ctx.Err()
				return
			case <-time.After(1 * time.Second):
			}
			if rv == "" {
				// It should never happen but just in case - the watch will restart from the
				// current "event horizon" rather than from the original resource version which may no
				// longer be available.
				args.Del("resourceVersion")
			} else {
				args.Set("resourceVersion", rv)
			}
		}
	}()
	return results
}

func (c *ResourceClient) Do(ctx context.Context, verb, groupVersion, namespace, resource, name, suffix string, args url.Values, expectedStatus int, request interface{}, response interface{}) error {
	return c.DoCheckResponse(ctx, verb, groupVersion, namespace, resource, name, suffix, args, expectedStatus, request, func(r io.Reader) error {
		// Consume body even if "response" is nil to enable connection reuse
		b, err := ioutil.ReadAll(io.LimitReader(r, maxResponseSize))
		if err != nil {
			return err
		}
		log.Printf("Server response for %s %s:\n%s", verb, c.formatUrl(groupVersion, namespace, resource, name, suffix, args), b)
		if response == nil {
			return nil
		}
		return json.Unmarshal(b, response)
	})
}

func (c *ResourceClient) DoCheckResponse(ctx context.Context, verb, groupVersion, namespace, resource, name, suffix string, args url.Values, expectedStatus int, request interface{}, f StreamHandler) error {
	return c.DoRequest(ctx, verb, groupVersion, namespace, resource, name, suffix, args, request, func(resp *http.Response) error {
		if resp.StatusCode != expectedStatus {
			msg := fmt.Sprintf("received bad status code %d", resp.StatusCode)
			b, err := ioutil.ReadAll(io.LimitReader(resp.Body, maxStatusResponseSize))
			if err != nil {
				return errors.New(msg)
			}
			se := StatusError{
				msg: msg,
			}
			log.Printf("Unexpected server response for %s %s: %d\n%s", verb, c.formatUrl(groupVersion, namespace, resource, name, suffix, args), resp.StatusCode, b)
			if json.Unmarshal(b, &se.status) != nil {
				return errors.New(msg)
			}
			return &se
		}
		// If this line is removed, program crashes with
		//panic: runtime error: invalid memory address or nil pointer dereference
		//[signal SIGSEGV: segmentation violation code=0x1 addr=0x0 pc=0x0]
		// https://github.com/golang/go/issues/17318
		log.Print(f)
		return f(resp.Body)
	})
}

func (c *ResourceClient) DoRequest(ctx context.Context, verb, groupVersion, namespace, resource, name, suffix string, args url.Values, request interface{}, f ResponseHandler) error {
	var body []byte
	if request != nil {
		var err error
		body, err = json.Marshal(request)
		if err != nil {
			return err
		}
	}
	reqUrl := c.formatUrl(groupVersion, namespace, resource, name, suffix, args)
	req, err := http.NewRequest(verb, reqUrl, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("unable to create http.Request: %v", err)
	}
	req = req.WithContext(ctx)
	if request != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.Agent)
	req.Header.Set("Authorization", "Bearer "+c.BearerToken)
	resp, err := c.Client.Do(req)
	if err != nil {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			return fmt.Errorf("request to %s failed: %v", reqUrl, err)
		}
	}
	defer resp.Body.Close()
	return f(resp)
}

func (c *ResourceClient) formatUrl(groupVersion, namespace, resource, name, suffix string, args url.Values) string {
	var prefix string
	if strings.ContainsRune(groupVersion, '/') {
		prefix = smith.DefaultAPIPath
	} else {
		prefix = smith.LegacyAPIPath
	}
	p := []string{prefix, groupVersion}
	if namespace != "" {
		p = append(p, "namespaces", namespace)
	}
	p = append(p, resource, name, suffix)
	u := url.URL{
		Scheme:   c.Scheme,
		Host:     c.HostPort,
		Path:     path.Join(p...),
		RawQuery: args.Encode(),
	}
	return u.String()
}

func NewInCluster() (*ResourceClient, error) {
	host, port := os.Getenv("KUBERNETES_SERVICE_HOST"), os.Getenv("KUBERNETES_SERVICE_PORT")
	if len(host) == 0 || len(port) == 0 {
		return nil, errors.New("unable to load in-cluster configuration, KUBERNETES_SERVICE_HOST and KUBERNETES_SERVICE_PORT must be defined")
	}
	token, err := ioutil.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/" + smith.ServiceAccountTokenKey)
	if err != nil {
		return nil, err
	}
	rootCA := "/var/run/secrets/kubernetes.io/serviceaccount/" + smith.ServiceAccountRootCAKey
	CAData, err := ioutil.ReadFile(rootCA)
	if err != nil {
		return nil, err
	}
	certPool := x509.NewCertPool()
	if !certPool.AppendCertsFromPEM(CAData) {
		log.Printf("Failed to add certificate from %s", rootCA)
	}
	return &ResourceClient{
		Scheme:      "https",
		HostPort:    net.JoinHostPort(host, port),
		BearerToken: string(token),
		Client: http.Client{
			Timeout: 10 * time.Minute,
			Transport: &http.Transport{
				Proxy:               http.ProxyFromEnvironment,
				TLSHandshakeTimeout: 10 * time.Second,
				TLSClientConfig: &tls.Config{
					// Can't use SSLv3 because of POODLE and BEAST
					// Can't use TLSv1.0 because of POODLE and BEAST using CBC cipher
					// Can't use TLSv1.1 because of RC4 cipher usage
					MinVersion: tls.VersionTLS12,
					RootCAs:    certPool,
					//InsecureSkipVerify: true,
				},
				DialContext: (&net.Dialer{
					Timeout:   30 * time.Second,
					KeepAlive: 30 * time.Second,
				}).DialContext,
			},
		},
	}, nil
}
