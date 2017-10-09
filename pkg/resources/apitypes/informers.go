package apitypes

import (
	"time"

	sc_v1a1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1alpha1"
	scClientset "github.com/kubernetes-incubator/service-catalog/pkg/client/clientset_generated/clientset"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	api_v1 "k8s.io/client-go/pkg/api/v1"
	apps_v1b1 "k8s.io/client-go/pkg/apis/apps/v1beta1"
	ext_v1b1 "k8s.io/client-go/pkg/apis/extensions/v1beta1"
	settings_v1a1 "k8s.io/client-go/pkg/apis/settings/v1alpha1"
	"k8s.io/client-go/tools/cache"
)

func ResourceInformers(mainClient kubernetes.Interface, scClient scClientset.Interface, namespace string, resyncPeriod time.Duration, enablePodPreset bool) map[schema.GroupVersionKind]cache.SharedIndexInformer {
	// Core API types
	infs := map[schema.GroupVersionKind]cache.SharedIndexInformer{
		ext_v1b1.SchemeGroupVersion.WithKind("Ingress"):     ingressInformer(mainClient, namespace, resyncPeriod),
		api_v1.SchemeGroupVersion.WithKind("Service"):       serviceInformer(mainClient, namespace, resyncPeriod),
		api_v1.SchemeGroupVersion.WithKind("ConfigMap"):     configMapInformer(mainClient, namespace, resyncPeriod),
		api_v1.SchemeGroupVersion.WithKind("Secret"):        secretInformer(mainClient, namespace, resyncPeriod),
		apps_v1b1.SchemeGroupVersion.WithKind("Deployment"): deploymentAppsInformer(mainClient, namespace, resyncPeriod),
	}
	if enablePodPreset {
		infs[settings_v1a1.SchemeGroupVersion.WithKind("PodPreset")] = podPresetInformer(mainClient, namespace, resyncPeriod)
	}

	// Service Catalog types
	if scClient != nil {
		infs[sc_v1a1.SchemeGroupVersion.WithKind("ServiceInstanceCredential")] = serviceInstanceCredentialInformer(scClient, namespace, resyncPeriod)
		infs[sc_v1a1.SchemeGroupVersion.WithKind("ServiceInstance")] = serviceInstanceInformer(scClient, namespace, resyncPeriod)
	}

	return infs
}

// TODO replace methods below with upstream functions https://github.com/kubernetes/kubernetes/issues/45939

func ingressInformer(mainClient kubernetes.Interface, namespace string, resyncPeriod time.Duration) cache.SharedIndexInformer {
	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options meta_v1.ListOptions) (runtime.Object, error) {
				return mainClient.ExtensionsV1beta1().Ingresses(namespace).List(options)
			},
			WatchFunc: func(options meta_v1.ListOptions) (watch.Interface, error) {
				return mainClient.ExtensionsV1beta1().Ingresses(namespace).Watch(options)
			},
		},
		&ext_v1b1.Ingress{},
		resyncPeriod,
		cache.Indexers{},
	)
}

func serviceInformer(mainClient kubernetes.Interface, namespace string, resyncPeriod time.Duration) cache.SharedIndexInformer {
	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options meta_v1.ListOptions) (runtime.Object, error) {
				return mainClient.CoreV1().Services(namespace).List(options)
			},
			WatchFunc: func(options meta_v1.ListOptions) (watch.Interface, error) {
				return mainClient.CoreV1().Services(namespace).Watch(options)
			},
		},
		&api_v1.Service{},
		resyncPeriod,
		cache.Indexers{},
	)
}

func configMapInformer(mainClient kubernetes.Interface, namespace string, resyncPeriod time.Duration) cache.SharedIndexInformer {
	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options meta_v1.ListOptions) (runtime.Object, error) {
				return mainClient.CoreV1().ConfigMaps(namespace).List(options)
			},
			WatchFunc: func(options meta_v1.ListOptions) (watch.Interface, error) {
				return mainClient.CoreV1().ConfigMaps(namespace).Watch(options)
			},
		},
		&api_v1.ConfigMap{},
		resyncPeriod,
		cache.Indexers{},
	)
}

func secretInformer(mainClient kubernetes.Interface, namespace string, resyncPeriod time.Duration) cache.SharedIndexInformer {
	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options meta_v1.ListOptions) (runtime.Object, error) {
				return mainClient.CoreV1().Secrets(namespace).List(options)
			},
			WatchFunc: func(options meta_v1.ListOptions) (watch.Interface, error) {
				return mainClient.CoreV1().Secrets(namespace).Watch(options)
			},
		},
		&api_v1.Secret{},
		resyncPeriod,
		cache.Indexers{},
	)
}

func deploymentAppsInformer(mainClient kubernetes.Interface, namespace string, resyncPeriod time.Duration) cache.SharedIndexInformer {
	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options meta_v1.ListOptions) (runtime.Object, error) {
				return mainClient.AppsV1beta1().Deployments(namespace).List(options)
			},
			WatchFunc: func(options meta_v1.ListOptions) (watch.Interface, error) {
				return mainClient.AppsV1beta1().Deployments(namespace).Watch(options)
			},
		},
		&apps_v1b1.Deployment{},
		resyncPeriod,
		cache.Indexers{},
	)
}

func podPresetInformer(mainClient kubernetes.Interface, namespace string, resyncPeriod time.Duration) cache.SharedIndexInformer {
	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options meta_v1.ListOptions) (runtime.Object, error) {
				return mainClient.SettingsV1alpha1().PodPresets(namespace).List(options)
			},
			WatchFunc: func(options meta_v1.ListOptions) (watch.Interface, error) {
				return mainClient.SettingsV1alpha1().PodPresets(namespace).Watch(options)
			},
		},
		&settings_v1a1.PodPreset{},
		resyncPeriod,
		cache.Indexers{},
	)
}

func serviceInstanceCredentialInformer(scClient scClientset.Interface, namespace string, resyncPeriod time.Duration) cache.SharedIndexInformer {
	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options meta_v1.ListOptions) (runtime.Object, error) {
				return scClient.ServicecatalogV1alpha1().ServiceInstanceCredentials(namespace).List(options)
			},
			WatchFunc: func(options meta_v1.ListOptions) (watch.Interface, error) {
				return scClient.ServicecatalogV1alpha1().ServiceInstanceCredentials(namespace).Watch(options)
			},
		},
		&sc_v1a1.ServiceInstanceCredential{},
		resyncPeriod,
		cache.Indexers{},
	)
}

func serviceInstanceInformer(scClient scClientset.Interface, namespace string, resyncPeriod time.Duration) cache.SharedIndexInformer {
	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options meta_v1.ListOptions) (runtime.Object, error) {
				return scClient.ServicecatalogV1alpha1().ServiceInstances(namespace).List(options)
			},
			WatchFunc: func(options meta_v1.ListOptions) (watch.Interface, error) {
				return scClient.ServicecatalogV1alpha1().ServiceInstances(namespace).Watch(options)
			},
		},
		&sc_v1a1.ServiceInstance{},
		resyncPeriod,
		cache.Indexers{},
	)
}
