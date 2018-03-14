package client

import (
	"time"

	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	smithClient_v1 "github.com/atlassian/smith/pkg/client/clientset_generated/clientset/typed/smith/v1"

	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
)

func TemplateRenderInformer(trc smithClient_v1.TemplateRendersGetter, namespace string, resyncPeriod time.Duration) cache.SharedIndexInformer {
	templateRendersApi := trc.TemplateRenders(namespace)
	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options meta_v1.ListOptions) (runtime.Object, error) {
				return templateRendersApi.List(options)
			},
			WatchFunc: func(options meta_v1.ListOptions) (watch.Interface, error) {
				return templateRendersApi.Watch(options)
			},
		},
		&smith_v1.TemplateRender{},
		resyncPeriod,
		cache.Indexers{})
}

func TemplateInformer(trc smithClient_v1.TemplatesGetter, namespace string, resyncPeriod time.Duration) cache.SharedIndexInformer {
	templateApi := trc.Templates(namespace)
	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options meta_v1.ListOptions) (runtime.Object, error) {
				return templateApi.List(options)
			},
			WatchFunc: func(options meta_v1.ListOptions) (watch.Interface, error) {
				return templateApi.Watch(options)
			},
		},
		&smith_v1.Template{},
		resyncPeriod,
		cache.Indexers{})
}
