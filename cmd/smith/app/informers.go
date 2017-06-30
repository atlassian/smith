package app

import (
	sc_v1a1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1alpha1"
	scClientset "github.com/kubernetes-incubator/service-catalog/pkg/client/clientset_generated/clientset"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	api_v1 "k8s.io/client-go/pkg/api/v1"
	apps_v1b1 "k8s.io/client-go/pkg/apis/apps/v1beta1"
	ext_v1b1 "k8s.io/client-go/pkg/apis/extensions/v1beta1"
	settings_v1a1 "k8s.io/client-go/pkg/apis/settings/v1alpha1"
	"k8s.io/client-go/tools/cache"
)

// TODO replace methods below with upstream functions https://github.com/kubernetes/kubernetes/issues/45939

func (a *App) ingressInformer(mainClient kubernetes.Interface) cache.SharedIndexInformer {
	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options meta_v1.ListOptions) (runtime.Object, error) {
				return mainClient.ExtensionsV1beta1().Ingresses(a.Namespace).List(options)
			},
			WatchFunc: func(options meta_v1.ListOptions) (watch.Interface, error) {
				return mainClient.ExtensionsV1beta1().Ingresses(a.Namespace).Watch(options)
			},
		},
		&ext_v1b1.Ingress{},
		a.ResyncPeriod,
		cache.Indexers{},
	)
}

func (a *App) serviceInformer(mainClient kubernetes.Interface) cache.SharedIndexInformer {
	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options meta_v1.ListOptions) (runtime.Object, error) {
				return mainClient.CoreV1().Services(a.Namespace).List(options)
			},
			WatchFunc: func(options meta_v1.ListOptions) (watch.Interface, error) {
				return mainClient.CoreV1().Services(a.Namespace).Watch(options)
			},
		},
		&api_v1.Service{},
		a.ResyncPeriod,
		cache.Indexers{},
	)
}

func (a *App) configMapInformer(mainClient kubernetes.Interface) cache.SharedIndexInformer {
	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options meta_v1.ListOptions) (runtime.Object, error) {
				return mainClient.CoreV1().ConfigMaps(a.Namespace).List(options)
			},
			WatchFunc: func(options meta_v1.ListOptions) (watch.Interface, error) {
				return mainClient.CoreV1().ConfigMaps(a.Namespace).Watch(options)
			},
		},
		&api_v1.ConfigMap{},
		a.ResyncPeriod,
		cache.Indexers{},
	)
}

func (a *App) secretInformer(mainClient kubernetes.Interface) cache.SharedIndexInformer {
	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options meta_v1.ListOptions) (runtime.Object, error) {
				return mainClient.CoreV1().Secrets(a.Namespace).List(options)
			},
			WatchFunc: func(options meta_v1.ListOptions) (watch.Interface, error) {
				return mainClient.CoreV1().Secrets(a.Namespace).Watch(options)
			},
		},
		&api_v1.Secret{},
		a.ResyncPeriod,
		cache.Indexers{},
	)
}

func (a *App) deploymentAppsInformer(mainClient kubernetes.Interface) cache.SharedIndexInformer {
	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options meta_v1.ListOptions) (runtime.Object, error) {
				return mainClient.AppsV1beta1().Deployments(a.Namespace).List(options)
			},
			WatchFunc: func(options meta_v1.ListOptions) (watch.Interface, error) {
				return mainClient.AppsV1beta1().Deployments(a.Namespace).Watch(options)
			},
		},
		&apps_v1b1.Deployment{},
		a.ResyncPeriod,
		cache.Indexers{},
	)
}

func (a *App) podPresetInformer(mainClient kubernetes.Interface) cache.SharedIndexInformer {
	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options meta_v1.ListOptions) (runtime.Object, error) {
				return mainClient.SettingsV1alpha1().PodPresets(a.Namespace).List(options)
			},
			WatchFunc: func(options meta_v1.ListOptions) (watch.Interface, error) {
				return mainClient.SettingsV1alpha1().PodPresets(a.Namespace).Watch(options)
			},
		},
		&settings_v1a1.PodPreset{},
		a.ResyncPeriod,
		cache.Indexers{},
	)
}

func (a *App) bindingInformer(scClient scClientset.Interface) cache.SharedIndexInformer {
	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options meta_v1.ListOptions) (runtime.Object, error) {
				return scClient.ServicecatalogV1alpha1().Bindings(a.Namespace).List(options)
			},
			WatchFunc: func(options meta_v1.ListOptions) (watch.Interface, error) {
				return scClient.ServicecatalogV1alpha1().Bindings(a.Namespace).Watch(options)
			},
		},
		&sc_v1a1.Binding{},
		a.ResyncPeriod,
		cache.Indexers{},
	)
}

func (a *App) instanceInformer(scClient scClientset.Interface) cache.SharedIndexInformer {
	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options meta_v1.ListOptions) (runtime.Object, error) {
				return scClient.ServicecatalogV1alpha1().Instances(a.Namespace).List(options)
			},
			WatchFunc: func(options meta_v1.ListOptions) (watch.Interface, error) {
				return scClient.ServicecatalogV1alpha1().Instances(a.Namespace).Watch(options)
			},
		},
		&sc_v1a1.Instance{},
		a.ResyncPeriod,
		cache.Indexers{},
	)
}
