package app

import (
	"github.com/atlassian/smith"

	scv1alpha1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1alpha1"
	scClientset "github.com/kubernetes-incubator/service-catalog/pkg/client/clientset_generated/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	apiv1 "k8s.io/client-go/pkg/api/v1"
	appsv1beta1 "k8s.io/client-go/pkg/apis/apps/v1beta1"
	extensions "k8s.io/client-go/pkg/apis/extensions/v1beta1"
	settings "k8s.io/client-go/pkg/apis/settings/v1alpha1"
	"k8s.io/client-go/tools/cache"
)

func (a *App) bundleInformer(bundleClient cache.Getter) cache.SharedIndexInformer {
	return cache.NewSharedIndexInformer(
		cache.NewListWatchFromClient(bundleClient, smith.BundleResourcePath, a.Namespace, fields.Everything()),
		&smith.Bundle{},
		a.ResyncPeriod,
		cache.Indexers{
			ByTprNameIndex: byTprNameIndex,
		})
}

func (a *App) deploymentExtInformer(mainClient kubernetes.Interface) cache.SharedIndexInformer {
	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return mainClient.ExtensionsV1beta1().Deployments(a.Namespace).List(options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return mainClient.ExtensionsV1beta1().Deployments(a.Namespace).Watch(options)
			},
		},
		&extensions.Deployment{},
		a.ResyncPeriod,
		cache.Indexers{},
	)
}

func (a *App) ingressInformer(mainClient kubernetes.Interface) cache.SharedIndexInformer {
	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return mainClient.ExtensionsV1beta1().Ingresses(a.Namespace).List(options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return mainClient.ExtensionsV1beta1().Ingresses(a.Namespace).Watch(options)
			},
		},
		&extensions.Ingress{},
		a.ResyncPeriod,
		cache.Indexers{},
	)
}

func (a *App) serviceInformer(mainClient kubernetes.Interface) cache.SharedIndexInformer {
	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return mainClient.CoreV1().Services(a.Namespace).List(options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return mainClient.CoreV1().Services(a.Namespace).Watch(options)
			},
		},
		&apiv1.Service{},
		a.ResyncPeriod,
		cache.Indexers{},
	)
}

func (a *App) configMapInformer(mainClient kubernetes.Interface) cache.SharedIndexInformer {
	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return mainClient.CoreV1().ConfigMaps(a.Namespace).List(options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return mainClient.CoreV1().ConfigMaps(a.Namespace).Watch(options)
			},
		},
		&apiv1.ConfigMap{},
		a.ResyncPeriod,
		cache.Indexers{},
	)
}

func (a *App) secretInformer(mainClient kubernetes.Interface) cache.SharedIndexInformer {
	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return mainClient.CoreV1().Secrets(a.Namespace).List(options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return mainClient.CoreV1().Secrets(a.Namespace).Watch(options)
			},
		},
		&apiv1.Secret{},
		a.ResyncPeriod,
		cache.Indexers{},
	)
}

func (a *App) deploymentAppsInformer(mainClient kubernetes.Interface) cache.SharedIndexInformer {
	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return mainClient.AppsV1beta1().Deployments(a.Namespace).List(options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return mainClient.AppsV1beta1().Deployments(a.Namespace).Watch(options)
			},
		},
		&appsv1beta1.Deployment{},
		a.ResyncPeriod,
		cache.Indexers{},
	)
}

func (a *App) podPresetInformer(mainClient kubernetes.Interface) cache.SharedIndexInformer {
	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return mainClient.SettingsV1alpha1().PodPresets(a.Namespace).List(options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return mainClient.SettingsV1alpha1().PodPresets(a.Namespace).Watch(options)
			},
		},
		&settings.PodPreset{},
		a.ResyncPeriod,
		cache.Indexers{},
	)
}

func (a *App) bindingInformer(scClient scClientset.Interface) cache.SharedIndexInformer {
	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return scClient.ServicecatalogV1alpha1().Bindings(a.Namespace).List(options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return scClient.ServicecatalogV1alpha1().Bindings(a.Namespace).Watch(options)
			},
		},
		&scv1alpha1.Binding{},
		a.ResyncPeriod,
		cache.Indexers{},
	)
}

func (a *App) instanceInformer(scClient scClientset.Interface) cache.SharedIndexInformer {
	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return scClient.ServicecatalogV1alpha1().Instances(a.Namespace).List(options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return scClient.ServicecatalogV1alpha1().Instances(a.Namespace).Watch(options)
			},
		},
		&scv1alpha1.Instance{},
		a.ResyncPeriod,
		cache.Indexers{},
	)
}
