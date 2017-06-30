package app

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/atlassian/smith"
	"github.com/atlassian/smith/pkg/cleanup"
	"github.com/atlassian/smith/pkg/client"
	"github.com/atlassian/smith/pkg/client/smart"
	"github.com/atlassian/smith/pkg/controller"
	"github.com/atlassian/smith/pkg/readychecker"
	"github.com/atlassian/smith/pkg/resources"
	"github.com/atlassian/smith/pkg/store"

	"github.com/ash2k/stager"
	sc_v1a1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1alpha1"
	scClientset "github.com/kubernetes-incubator/service-catalog/pkg/client/clientset_generated/clientset"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	api_v1 "k8s.io/client-go/pkg/api/v1"
	apps_v1b1 "k8s.io/client-go/pkg/apis/apps/v1beta1"
	ext_v1b1 "k8s.io/client-go/pkg/apis/extensions/v1beta1"
	settings_v1a1 "k8s.io/client-go/pkg/apis/settings/v1alpha1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

type App struct {
	RestConfig           *rest.Config
	ServiceCatalogConfig *rest.Config
	ResyncPeriod         time.Duration
	Namespace            string
	DisablePodPreset     bool
	Workers              int
}

func (a *App) Run(ctx context.Context) error {
	// 0. Clients
	clientset, err := kubernetes.NewForConfig(a.RestConfig)
	if err != nil {
		return err
	}
	bundleClient, err := client.BundleClient(a.RestConfig, client.BundleScheme())
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
	sc := smart.NewClient(a.RestConfig, a.ServiceCatalogConfig, clientset, scClient)
	scheme, err := FullScheme(a.ServiceCatalogConfig != nil)
	if err != nil {
		return err
	}

	stgr := stager.New()
	defer stgr.Shutdown()

	// 1. Multi store
	stage := stgr.NextStage()
	multiStore := store.NewMulti(scheme.DeepCopy)
	stage.StartWithContext(multiStore.Run)

	// 1.5. Informers
	bundleInf := client.BundleInformer(bundleClient, a.Namespace, a.ResyncPeriod)
	multiStore.AddInformer(smith.BundleGVK, bundleInf)

	resourceInfs, tprInf := a.resourceInformers(clientset, scClient)
	multiStore.AddInformer(tprGVK, tprInf)
	stage = stgr.NextStage()
	stage.StartWithChannel(tprInf.Run) // Must be after store.AddInformer()

	// We must wait for tprInf to populate its cache to avoid reading from an empty cache
	// in Ready Checker and in EnsureTprExists().
	if !cache.WaitForCacheSync(ctx.Done(), tprInf.HasSynced) {
		return errors.New("wait for TPR Informer was cancelled")
	}

	// 2. Ready Checker
	readyTypes := []map[schema.GroupKind]readychecker.IsObjectReady{readychecker.MainKnownTypes}
	if a.ServiceCatalogConfig != nil {
		readyTypes = append(readyTypes, readychecker.ServiceCatalogKnownTypes)
	}
	rc := readychecker.New(&store.Tpr{Store: multiStore}, readyTypes...)

	// 3. Object cleanup
	cleanupTypes := []map[schema.GroupKind]cleanup.SpecCleanup{cleanup.MainKnownTypes}
	if a.ServiceCatalogConfig != nil {
		cleanupTypes = append(cleanupTypes, cleanup.ServiceCatalogKnownTypes)
	}
	oc := cleanup.New(cleanupTypes...)

	// 3. Ensure ThirdPartyResource Bundle exists
	err = retryUntilSuccessOrDone(ctx, func() error {
		return resources.EnsureTprExists(ctx, clientset, multiStore, resources.BundleTpr())
	}, func(e error) bool {
		// TODO be smarter about what is retried
		log.Printf("Failed to create resource %s: %v", smith.BundleResourceName, e)
		return false
	})
	if err != nil {
		return err
	}

	// 4. Controller
	bs, err := store.NewBundle(bundleInf, multiStore, scheme.DeepCopy)
	if err != nil {
		return err
	}

	queue := workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "bundle")
	cntrlr := controller.New(bundleInf, tprInf, bundleClient, bs, sc, rc, scheme, multiStore, queue, a.Workers, oc, a.ResyncPeriod, resourceInfs)

	// Add all to Multi store
	for gvk, inf := range resourceInfs {
		multiStore.AddInformer(gvk, inf)
		stage.StartWithChannel(inf.Run) // Must be after store.AddInformer()
	}
	stage.StartWithChannel(bundleInf.Run)

	cntrlr.Run(ctx)
	return ctx.Err()
}

func (a *App) resourceInformers(mainClient kubernetes.Interface, scClient scClientset.Interface) (map[schema.GroupVersionKind]cache.SharedIndexInformer, cache.SharedIndexInformer) {
	mainSif := informers.NewSharedInformerFactory(mainClient, a.ResyncPeriod)

	// Core API types
	infs := map[schema.GroupVersionKind]cache.SharedIndexInformer{
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

	tprInf := mainSif.Extensions().V1beta1().ThirdPartyResources().Informer()

	return infs, tprInf
}
