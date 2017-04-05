package app

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/atlassian/smith"
	"github.com/atlassian/smith/pkg/processor"
	"github.com/atlassian/smith/pkg/readychecker"
	"github.com/atlassian/smith/pkg/resources"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	apiv1 "k8s.io/client-go/pkg/api/v1"
	extensions "k8s.io/client-go/pkg/apis/extensions/v1beta1"
	settings "k8s.io/client-go/pkg/apis/settings/v1alpha1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

const (
	ResyncPeriod     = 1 * time.Minute
	PodPresetEnabled = true
)

type App struct {
	RestConfig *rest.Config
}

func (a *App) Run(ctx context.Context) error {
	clientset, err := kubernetes.NewForConfig(a.RestConfig)
	if err != nil {
		return err
	}
	bundleScheme := resources.GetBundleScheme()
	bundleClient, err := resources.GetBundleTprClient(a.RestConfig, bundleScheme)
	if err != nil {
		return err
	}

	clients := dynamic.NewClientPool(a.RestConfig, nil, dynamic.LegacyAPIPathResolverFunc)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// 1. Informers

	informerFactory := informers.NewSharedInformerFactory(clientset, ResyncPeriod)
	tprInf := informerFactory.Extensions().V1beta1().ThirdPartyResources().Informer()
	deploymentInf := informerFactory.Extensions().V1beta1().Deployments().Informer()
	ingressInf := informerFactory.Extensions().V1beta1().Ingresses().Informer()
	serviceInf := informerFactory.Core().V1().Services().Informer()
	configMapInf := informerFactory.Core().V1().ConfigMaps().Informer()
	secretInf := informerFactory.Core().V1().Secrets().Informer()
	var podPresetInf cache.SharedIndexInformer
	if PodPresetEnabled {
		podPresetInf = informerFactory.Settings().V1alpha1().PodPresets().Informer()
	}
	bundleInf := bundleInformer(bundleClient)

	// 1.5 Store
	store := resources.NewStore(bundleScheme.DeepCopy)

	var wgStore sync.WaitGroup
	defer wgStore.Wait() // await store termination

	ctxStore, cancelStore := context.WithCancel(context.Background())
	defer cancelStore() // signal store to stop
	wgStore.Add(1)
	go store.Run(ctxStore, &wgStore)

	store.AddInformer(smith.TprGVK, tprInf)
	store.AddInformer(extensions.SchemeGroupVersion.WithKind("Deployment"), deploymentInf)
	store.AddInformer(extensions.SchemeGroupVersion.WithKind("Ingress"), ingressInf)
	store.AddInformer(apiv1.SchemeGroupVersion.WithKind("Service"), serviceInf)
	store.AddInformer(apiv1.SchemeGroupVersion.WithKind("ConfigMap"), configMapInf)
	store.AddInformer(apiv1.SchemeGroupVersion.WithKind("Secret"), secretInf)
	if PodPresetEnabled {
		store.AddInformer(settings.SchemeGroupVersion.WithKind("PodPreset"), podPresetInf)
	}
	store.AddInformer(smith.BundleGVK, bundleInf)

	informerFactory.Start(ctx.Done()) // Must be after store.AddInformer()

	// We must wait for tprInf to populate its cache to avoid reading from an empty cache
	// in Ready Checker.
	if !cache.WaitForCacheSync(ctx.Done(), tprInf.HasSynced) {
		return errors.New("wait for TPR Informer was cancelled")
	}

	// 2. Ready Checker

	rc := &readychecker.ReadyChecker{
		Store: &tprStore{
			store: store,
		},
	}

	// 3. Processor

	bp := processor.New(bundleClient, clients, rc, bundleScheme.DeepCopy, store)
	var wg sync.WaitGroup
	defer wg.Wait() // await termination
	wg.Add(1)
	go bp.Run(ctx, &wg)
	defer cancel() // cancel ctx to signal done to processor (and everything else)

	// 4. Ensure ThirdPartyResource TEMPLATE exists
	bundleTpr := &extensions.ThirdPartyResource{
		ObjectMeta: metav1.ObjectMeta{
			Name: smith.BundleResourceName,
		},
		Description: "Smith resource manager",
		Versions: []extensions.APIVersion{
			{Name: smith.BundleResourceVersion},
		},
	}
	err = retryUntilSuccessOrDone(ctx, func() error {
		return resources.EnsureTprExists(ctx, clientset, store, bundleTpr)
	}, func(e error) bool {
		// TODO be smarter about what is retried
		log.Printf("Failed to create resource %s: %v", smith.BundleResourceName, e)
		return false
	})
	if err != nil {
		return err
	}

	// 5. Watch Bundles

	bundleInf.AddEventHandler(&bundleEventHandler{
		ctx:       ctx,
		processor: bp,
		deepCopy:  bundleScheme.DeepCopy,
	})

	go bundleInf.Run(ctx.Done())

	// We must wait for bundleInf to populate its cache to avoid reading from an empty cache
	// in case of resource-generated events.
	if !cache.WaitForCacheSync(ctx.Done(), bundleInf.HasSynced) {
		return errors.New("wait for Bundle Informer was cancelled")
	}

	sl := bundleStore{
		store: store,
	}
	reh := &resourceEventHandler{
		ctx:         ctx,
		processor:   bp,
		name2bundle: sl.Get,
	}

	// 6. Watch supported built-in resource types

	deploymentInf.AddEventHandler(reh)
	ingressInf.AddEventHandler(reh)
	serviceInf.AddEventHandler(reh)
	configMapInf.AddEventHandler(reh)
	secretInf.AddEventHandler(reh)
	if PodPresetEnabled {
		podPresetInf.AddEventHandler(reh)
	}

	// 7. Watch Third Party Resources to add watches for supported ones

	tprInf.AddEventHandler(newTprEventHandler(ctx, reh, clients, store, ResyncPeriod))

	<-ctx.Done()
	return ctx.Err()
}

func bundleInformer(bundleClient cache.Getter) cache.SharedIndexInformer {
	return cache.NewSharedIndexInformer(
		cache.NewListWatchFromClient(bundleClient, smith.BundleResourcePath, metav1.NamespaceAll, fields.Everything()),
		&smith.Bundle{},
		ResyncPeriod,
		cache.Indexers{})
}
