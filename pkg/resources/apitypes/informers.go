package apitypes

import (
	"time"

	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	scClientset "github.com/kubernetes-incubator/service-catalog/pkg/client/clientset_generated/clientset"
	sc_v1b1inf "github.com/kubernetes-incubator/service-catalog/pkg/client/informers_generated/externalversions/servicecatalog/v1beta1"
	apps_v1b2 "k8s.io/api/apps/v1beta2"
	core_v1 "k8s.io/api/core/v1"
	ext_v1b1 "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	apps_v1b2inf "k8s.io/client-go/informers/apps/v1beta2"
	core_v1inf "k8s.io/client-go/informers/core/v1"
	ext_v1b1inf "k8s.io/client-go/informers/extensions/v1beta1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

func ResourceInformers(mainClient kubernetes.Interface, scClient scClientset.Interface, namespace string, resyncPeriod time.Duration) map[schema.GroupVersionKind]cache.SharedIndexInformer {
	// Core API types
	infs := map[schema.GroupVersionKind]cache.SharedIndexInformer{
		ext_v1b1.SchemeGroupVersion.WithKind("Ingress"):     ext_v1b1inf.NewIngressInformer(mainClient, namespace, resyncPeriod, cache.Indexers{}),
		core_v1.SchemeGroupVersion.WithKind("Service"):      core_v1inf.NewServiceInformer(mainClient, namespace, resyncPeriod, cache.Indexers{}),
		core_v1.SchemeGroupVersion.WithKind("ConfigMap"):    core_v1inf.NewConfigMapInformer(mainClient, namespace, resyncPeriod, cache.Indexers{}),
		core_v1.SchemeGroupVersion.WithKind("Secret"):       core_v1inf.NewSecretInformer(mainClient, namespace, resyncPeriod, cache.Indexers{}),
		apps_v1b2.SchemeGroupVersion.WithKind("Deployment"): apps_v1b2inf.NewDeploymentInformer(mainClient, namespace, resyncPeriod, cache.Indexers{}),
	}

	// Service Catalog types
	if scClient != nil {
		infs[sc_v1b1.SchemeGroupVersion.WithKind("ServiceBinding")] = sc_v1b1inf.NewServiceBindingInformer(scClient, namespace, resyncPeriod, cache.Indexers{})
		infs[sc_v1b1.SchemeGroupVersion.WithKind("ServiceInstance")] = sc_v1b1inf.NewServiceInstanceInformer(scClient, namespace, resyncPeriod, cache.Indexers{})
	}

	return infs
}
