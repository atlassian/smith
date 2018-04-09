package controller

import (
	"context"
	"flag"
	"time"

	smithClientset "github.com/atlassian/smith/pkg/client/clientset_generated/clientset"
	scClientset "github.com/kubernetes-incubator/service-catalog/pkg/client/clientset_generated/clientset"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	apiExtClientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

type Descriptor struct {
	Gvk schema.GroupVersionKind
}

type Constructor interface {
	AddFlags(*flag.FlagSet)
	New(*Config, *Context) (Interface, error)
	Describe() Descriptor
}

type Interface interface {
	Run(context.Context)
	Process(*ProcessContext) (retriable bool, err error)
}

type SmartClient interface {
	ForGVK(gvk schema.GroupVersionKind, namespace string) (dynamic.ResourceInterface, error)
}

type WorkQueueProducer interface {
	// Add adds an item to the workqueue.
	Add(QueueKey)
}

type ProcessContext struct {
	Logger *zap.Logger
	Object runtime.Object
}

type QueueKey struct {
	Namespace string
	Name      string
}

type Config struct {
	Logger       *zap.Logger
	Namespace    string
	ResyncPeriod time.Duration

	MainClient   kubernetes.Interface
	ApiExtClient apiExtClientset.Interface
	ScClient     scClientset.Interface
	SmithClient  smithClientset.Interface
	SmartClient  SmartClient
}

type Context struct {
	Informers map[schema.GroupVersionKind]cache.SharedIndexInformer
	// Will contain all controllers once Generic controller constructs them
	Controllers map[schema.GroupVersionKind]Interface
	WorkQueue   WorkQueueProducer
}

func (c *Context) RegisterInformer(gvk schema.GroupVersionKind, inf cache.SharedIndexInformer) error {
	if _, ok := c.Informers[gvk]; ok {
		return errors.New("informer with this GVK has been registered already")
	}
	if c.Informers == nil {
		c.Informers = make(map[schema.GroupVersionKind]cache.SharedIndexInformer)
	}
	c.Informers[gvk] = inf
	return nil
}

func (c *Context) MainInformer(config *Config, gvk schema.GroupVersionKind, f func(kubernetes.Interface, string, time.Duration, cache.Indexers) cache.SharedIndexInformer) (cache.SharedIndexInformer, error) {
	inf := c.Informers[gvk]
	if inf == nil {
		inf = f(config.MainClient, config.Namespace, config.ResyncPeriod, cache.Indexers{})
		err := c.RegisterInformer(gvk, inf)
		if err != nil {
			return nil, err
		}
	}
	return inf, nil
}

func (c *Context) SmithInformer(config *Config, gvk schema.GroupVersionKind, f func(smithClientset.Interface, string, time.Duration) cache.SharedIndexInformer) (cache.SharedIndexInformer, error) {
	inf := c.Informers[gvk]
	if inf == nil {
		inf = f(config.SmithClient, config.Namespace, config.ResyncPeriod)
		err := c.RegisterInformer(gvk, inf)
		if err != nil {
			return nil, err
		}
	}
	return inf, nil
}

func (c *Context) ApiExtensionsInformer(config *Config, gvk schema.GroupVersionKind, f func(apiExtClientset.Interface, time.Duration, cache.Indexers) cache.SharedIndexInformer) (cache.SharedIndexInformer, error) {
	inf := c.Informers[gvk]
	if inf == nil {
		inf = f(config.ApiExtClient, config.ResyncPeriod, cache.Indexers{})
		err := c.RegisterInformer(gvk, inf)
		if err != nil {
			return nil, err
		}
	}
	return inf, nil
}

func (c *Context) SvcCatClusterInformer(config *Config, gvk schema.GroupVersionKind, f func(scClientset.Interface, time.Duration, cache.Indexers) cache.SharedIndexInformer) (cache.SharedIndexInformer, error) {
	inf := c.Informers[gvk]
	if inf == nil {
		inf = f(config.ScClient, config.ResyncPeriod, cache.Indexers{})
		err := c.RegisterInformer(gvk, inf)
		if err != nil {
			return nil, err
		}
	}
	return inf, nil
}

func (c *Context) SvcCatInformer(config *Config, gvk schema.GroupVersionKind, f func(scClientset.Interface, string, time.Duration, cache.Indexers) cache.SharedIndexInformer) (cache.SharedIndexInformer, error) {
	inf := c.Informers[gvk]
	if inf == nil {
		inf = f(config.ScClient, config.Namespace, config.ResyncPeriod, cache.Indexers{})
		err := c.RegisterInformer(gvk, inf)
		if err != nil {
			return nil, err
		}
	}
	return inf, nil
}
