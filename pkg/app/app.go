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
	"github.com/atlassian/smith/pkg/util"

	sc_v1a1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1alpha1"
	scClientset "github.com/kubernetes-incubator/service-catalog/pkg/client/clientset_generated/clientset"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	api_v1 "k8s.io/client-go/pkg/api/v1"
	apps_v1b1 "k8s.io/client-go/pkg/apis/apps/v1beta1"
	ext_v1b1 "k8s.io/client-go/pkg/apis/extensions/v1beta1"
	settings_v1a1 "k8s.io/client-go/pkg/apis/settings/v1alpha1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

type App struct {
	RestConfig           *rest.Config
	ServiceCatalogConfig *rest.Config
	ResyncPeriod         time.Duration
	Namespace            string
	DisablePodPreset     bool
}

func (a *App) Run(ctx context.Context) error {
	// 0. Clients
	clientset, err := kubernetes.NewForConfig(a.RestConfig)
	if err != nil {
		return err
	}
	bundleClient, err := resources.GetBundleTprClient(a.RestConfig, resources.BundleScheme())
	if err != nil {
		return err
	}
	var scClient scClientset.Interface
	if a.ServiceCatalogConfig != nil {
		scClient, err = scClientset.NewForConfig(a.ServiceCatalogConfig)
		if err != nil {
			return err
		}
	}
	sc := resources.NewSmartClient(a.RestConfig, a.ServiceCatalogConfig, clientset, scClient)
	scheme, err := resources.FullScheme(a.ServiceCatalogConfig != nil)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// 1. Store
	store := resources.NewStore(scheme.DeepCopy)

	var wgStore sync.WaitGroup
	defer wgStore.Wait() // await store termination

	ctxStore, cancelStore := context.WithCancel(context.Background())
	defer cancelStore() // signal store to stop
	util.StartAsync(ctxStore, &wgStore, store.Run)

	// 1.5. Informers
	bundleInf := a.bundleInformer(bundleClient)
	store.AddInformer(smith.BundleGVK, bundleInf)

	infs := a.resourceInformers(ctx, store, clientset, scClient)
	tprInf := infs[tprGVK]
	delete(infs, tprGVK) // To avoid adding generic event handler later

	// We must wait for tprInf to populate its cache to avoid reading from an empty cache
	// in Ready Checker.
	if !cache.WaitForCacheSync(ctx.Done(), tprInf.HasSynced) {
		return errors.New("wait for TPR Informer was cancelled")
	}

	// 2. Ready Checker
	types := []map[schema.GroupKind]readychecker.IsObjectReady{readychecker.MainKnownTypes}
	if a.ServiceCatalogConfig != nil {
		types = append(types, readychecker.ServiceCatalogKnownTypes)
	}
	rc := readychecker.New(&tprStore{store: store}, types...)

	// 3. Processor

	bp := processor.New(bundleClient, sc, rc, scheme.DeepCopy, store)
	var wg sync.WaitGroup
	defer wg.Wait() // await termination
	util.StartAsync(ctx, &wg, bp.Run)
	defer cancel() // cancel ctx to signal done to processor (and everything else)

	// 4. Ensure ThirdPartyResource Bundle exists
	bundleTpr := &ext_v1b1.ThirdPartyResource{
		ObjectMeta: meta_v1.ObjectMeta{
			Name: smith.BundleResourceName,
		},
		Description: "Smith resource manager",
		Versions: []ext_v1b1.APIVersion{
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

	tprInf.AddEventHandler(newTprEventHandler(ctx, reh, sc, store, bp, bs, a.ResyncPeriod))

	<-ctx.Done()
	return ctx.Err()
}

func (a *App) resourceInformers(ctx context.Context, store *resources.Store, mainClient kubernetes.Interface, scClient scClientset.Interface) map[schema.GroupVersionKind]cache.SharedIndexInformer {
	mainSif := informers.NewSharedInformerFactory(mainClient, a.ResyncPeriod)

	// Core API types
	infs := map[schema.GroupVersionKind]cache.SharedIndexInformer{
		tprGVK: mainSif.Extensions().V1beta1().ThirdPartyResources().Informer(),
		ext_v1b1.SchemeGroupVersion.WithKind("Deployment"):  a.deploymentExtInformer(mainClient),
		ext_v1b1.SchemeGroupVersion.WithKind("Ingress"):     a.ingressInformer(mainClient),
		api_v1.SchemeGroupVersion.WithKind("Service"):       a.serviceInformer(mainClient),
		api_v1.SchemeGroupVersion.WithKind("ConfigMap"):     a.configMapInformer(mainClient),
		api_v1.SchemeGroupVersion.WithKind("Secret"):        a.secretInformer(mainClient),
		apps_v1b1.SchemeGroupVersion.WithKind("Deployment"): a.deploymentAppsInformer(mainClient),
	}
	if !a.DisablePodPreset {
		infs[settings_v1a1.SchemeGroupVersion.WithKind("PodPreset")] = a.podPresetInformer(mainClient)
	}

	// Service Catalog types
	if scClient != nil {
		infs[sc_v1a1.SchemeGroupVersion.WithKind("Binding")] = a.bindingInformer(scClient)
		infs[sc_v1a1.SchemeGroupVersion.WithKind("Instance")] = a.instanceInformer(scClient)
	}

	// Add all to Store
	for gvk, inf := range infs {
		store.AddInformer(gvk, inf)
		go inf.Run(ctx.Done()) // Must be after store.AddInformer()
	}

	return infs
}
