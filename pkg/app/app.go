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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	apiv1 "k8s.io/client-go/pkg/api/v1"
	appsv1beta1 "k8s.io/client-go/pkg/apis/apps/v1beta1"
	extensions "k8s.io/client-go/pkg/apis/extensions/v1beta1"
	settings "k8s.io/client-go/pkg/apis/settings/v1alpha1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

type App struct {
	RestConfig       *rest.Config
	ResyncPeriod     time.Duration
	DisablePodPreset bool
}

func (a *App) Run(ctx context.Context) error {
	clientset, err := kubernetes.NewForConfig(a.RestConfig)
	if err != nil {
		return err
	}
	scheme := runtime.NewScheme()
	scheme.AddUnversionedTypes(apiv1.SchemeGroupVersion, &metav1.Status{})
	smith.AddToScheme(scheme)

	bundleClient, err := resources.GetBundleTprClient(a.RestConfig, scheme)
	if err != nil {
		return err
	}

	clients := dynamic.NewClientPool(a.RestConfig, nil, dynamic.LegacyAPIPathResolverFunc)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// 1. Store
	store := resources.NewStore(scheme.DeepCopy)

	var wgStore sync.WaitGroup
	defer wgStore.Wait() // await store termination

	ctxStore, cancelStore := context.WithCancel(context.Background())
	defer cancelStore() // signal store to stop
	wgStore.Add(1)
	go store.Run(ctxStore, wgStore.Done)

	// 1.5. Informers
	bundleInf := a.bundleInformer(bundleClient)
	store.AddInformer(smith.BundleGVK, bundleInf)

	infs, err := a.resourceInformers(ctx, store, clientset)
	if err != nil {
		return err
	}
	tprInf := infs[smith.TprGVK]
	delete(infs, smith.TprGVK) // To avoid adding generic event handler later

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

	bp := processor.New(bundleClient, clients, rc, scheme.DeepCopy, store)
	var wg sync.WaitGroup
	defer wg.Wait() // await termination
	wg.Add(1)
	go bp.Run(ctx, wg.Done)
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
		deepCopy:  scheme.DeepCopy,
	})

	go bundleInf.Run(ctx.Done())

	// We must wait for bundleInf to populate its cache to avoid reading from an empty cache
	// in case of resource-generated events.
	if !cache.WaitForCacheSync(ctx.Done(), bundleInf.HasSynced) {
		return errors.New("wait for Bundle Informer was cancelled")
	}

	bs := &bundleStore{
		store:         store,
		bundleByIndex: bundleInf.GetIndexer().ByIndex,
		deepCopy:      scheme.DeepCopy,
	}
	reh := &resourceEventHandler{
		ctx:         ctx,
		processor:   bp,
		name2bundle: bs.Get,
	}

	// 6. Watch supported built-in resource types

	for _, inf := range infs {
		inf.AddEventHandler(reh)
	}

	// 7. Watch Third Party Resources to add watches for supported ones

	tprInf.AddEventHandler(newTprEventHandler(ctx, reh, clients, store, bp, bs, a.ResyncPeriod))

	<-ctx.Done()
	return ctx.Err()
}

func (a *App) bundleInformer(bundleClient cache.Getter) cache.SharedIndexInformer {
	return cache.NewSharedIndexInformer(
		cache.NewListWatchFromClient(bundleClient, smith.BundleResourcePath, metav1.NamespaceAll, fields.Everything()),
		&smith.Bundle{},
		a.ResyncPeriod,
		cache.Indexers{
			ByTprNameIndex: byTprNameIndex,
		})
}

func (a *App) resourceInformers(ctx context.Context, store *resources.Store, mainClient kubernetes.Interface) (map[schema.GroupVersionKind]cache.SharedIndexInformer, error) {
	mainSif := informers.NewSharedInformerFactory(mainClient, a.ResyncPeriod)

	// Core API types
	infs := map[schema.GroupVersionKind]cache.SharedIndexInformer{
		smith.TprGVK: mainSif.Extensions().V1beta1().ThirdPartyResources().Informer(),
		extensions.SchemeGroupVersion.WithKind("Deployment"):  mainSif.Extensions().V1beta1().Deployments().Informer(),
		extensions.SchemeGroupVersion.WithKind("Ingress"):     mainSif.Extensions().V1beta1().Ingresses().Informer(),
		apiv1.SchemeGroupVersion.WithKind("Service"):          mainSif.Core().V1().Services().Informer(),
		apiv1.SchemeGroupVersion.WithKind("ConfigMap"):        mainSif.Core().V1().ConfigMaps().Informer(),
		apiv1.SchemeGroupVersion.WithKind("Secret"):           mainSif.Core().V1().Secrets().Informer(),
		appsv1beta1.SchemeGroupVersion.WithKind("Deployment"): mainSif.Apps().V1beta1().Deployments().Informer(),
	}
	if !a.DisablePodPreset {
		infs[settings.SchemeGroupVersion.WithKind("PodPreset")] = mainSif.Settings().V1alpha1().PodPresets().Informer()
	}

	// Add all to Store
	for gvk, inf := range infs {
		store.AddInformer(gvk, inf)
	}

	// Start informers
	mainSif.Start(ctx.Done()) // Must be after store.AddInformer()
	return infs, nil
}
