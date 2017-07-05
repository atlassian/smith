package app

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/atlassian/smith"
	"github.com/atlassian/smith/pkg/cleanup"
	clean_types "github.com/atlassian/smith/pkg/cleanup/types"
	"github.com/atlassian/smith/pkg/client"
	"github.com/atlassian/smith/pkg/client/smart"
	"github.com/atlassian/smith/pkg/controller"
	"github.com/atlassian/smith/pkg/readychecker"
	ready_types "github.com/atlassian/smith/pkg/readychecker/types"
	"github.com/atlassian/smith/pkg/resources"
	"github.com/atlassian/smith/pkg/speccheck"
	"github.com/atlassian/smith/pkg/store"

	"github.com/ash2k/stager"
	sc_v1a1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1alpha1"
	scClientset "github.com/kubernetes-incubator/service-catalog/pkg/client/clientset_generated/clientset"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
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

var (
	tprCreationBackoff = wait.Backoff{
		Duration: 1 * time.Second,
		Factor:   1.2,
		Jitter:   0.1,
		Steps:    7,
	}
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
	// Clients
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

	// Stager will perform ordered, graceful shutdown
	stgr := stager.New()
	defer stgr.Shutdown()

	// Multi store
	stage := stgr.NextStage()
	multiStore := store.NewMulti(scheme.DeepCopy)
	stage.StartWithContext(multiStore.Run)

	// Informers
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

	// Ensure ThirdPartyResource Bundle exists
	err = wait.ExponentialBackoff(tprCreationBackoff, func() (bool /*done*/, error) {
		if err := resources.EnsureTprExists(ctx, clientset, multiStore, resources.BundleTpr()); err != nil {
			// TODO be smarter about what is retried
			if err == context.Canceled || err == context.DeadlineExceeded {
				return true, err
			}
			log.Printf("Failed to create TPR %s: %v", smith.BundleResourceName, err)
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		return err
	}

	// Controller
	bs, err := store.NewBundle(bundleInf, multiStore, scheme.DeepCopy)
	if err != nil {
		return err
	}
	cntrlr := a.controller(bundleInf, tprInf, bundleClient, bs, sc, scheme, multiStore, resourceInfs)

	// Add all informers to Multi store and start them
	for gvk, inf := range resourceInfs {
		multiStore.AddInformer(gvk, inf)
		stage.StartWithChannel(inf.Run) // Must be after AddInformer()
	}
	stage.StartWithChannel(bundleInf.Run)

	cntrlr.Run(ctx)
	return ctx.Err()
}

func (a *App) controller(bundleInf, tprInf cache.SharedIndexInformer, bundleClient *rest.RESTClient, bundleStore controller.BundleStore,
	sc smith.SmartClient, scheme *runtime.Scheme, cStore controller.Store, resourceInfs map[schema.GroupVersionKind]cache.SharedIndexInformer) *controller.BundleController {

	// Ready Checker
	readyTypes := []map[schema.GroupKind]readychecker.IsObjectReady{ready_types.MainKnownTypes}
	if a.ServiceCatalogConfig != nil {
		readyTypes = append(readyTypes, ready_types.ServiceCatalogKnownTypes)
	}
	rc := readychecker.New(&store.Tpr{Store: cStore}, readyTypes...)

	// Object cleanup
	cleanupTypes := []map[schema.GroupKind]cleanup.SpecCleanup{clean_types.MainKnownTypes}
	if a.ServiceCatalogConfig != nil {
		cleanupTypes = append(cleanupTypes, clean_types.ServiceCatalogKnownTypes)
	}
	oc := cleanup.New(cleanupTypes...)

	// Spec check
	specCheck := &speccheck.SpecCheck{
		Scheme:  scheme,
		Cleaner: oc,
	}
	queue := workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "bundle")

	return controller.New(bundleInf, tprInf, bundleClient, bundleStore, sc, rc, scheme, cStore, specCheck, queue, a.Workers, a.ResyncPeriod, resourceInfs)
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
