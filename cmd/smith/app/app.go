package app

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/atlassian/smith"
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/smith/pkg/cleanup"
	clean_types "github.com/atlassian/smith/pkg/cleanup/types"
	"github.com/atlassian/smith/pkg/client"
	smithClientset "github.com/atlassian/smith/pkg/client/clientset_generated/clientset"
	smithClient_v1 "github.com/atlassian/smith/pkg/client/clientset_generated/clientset/typed/smith/v1"
	"github.com/atlassian/smith/pkg/client/smart"
	"github.com/atlassian/smith/pkg/controller"
	"github.com/atlassian/smith/pkg/plugin"
	"github.com/atlassian/smith/pkg/readychecker"
	ready_types "github.com/atlassian/smith/pkg/readychecker/types"
	"github.com/atlassian/smith/pkg/resources"
	"github.com/atlassian/smith/pkg/resources/apitypes"
	"github.com/atlassian/smith/pkg/speccheck"
	"github.com/atlassian/smith/pkg/store"

	"github.com/ash2k/stager"
	scClientset "github.com/kubernetes-incubator/service-catalog/pkg/client/clientset_generated/clientset"
	"github.com/pkg/errors"
	apiext_v1b1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	crdClientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	crdInformers "k8s.io/apiextensions-apiserver/pkg/client/informers/externalversions"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

const (
	defaultResyncPeriod = 20 * time.Minute
)

type App struct {
	RestConfig           *rest.Config
	ServiceCatalogConfig *rest.Config
	ResyncPeriod         time.Duration
	Namespace            string
	Plugins              []plugin.NewFunc
	Workers              int
	DisablePodPreset     bool
}

func (a *App) Run(ctx context.Context) error {
	// Plugins
	plugins, err := a.loadPlugins()
	if err != nil {
		return err
	}
	for pluginName := range plugins {
		log.Printf("Loaded plugin: %q", pluginName)
	}

	// Clients
	clientset, err := kubernetes.NewForConfig(a.RestConfig)
	if err != nil {
		return err
	}
	bundleClient, err := smithClientset.NewForConfig(a.RestConfig)
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
	crdClient, err := crdClientset.NewForConfig(a.RestConfig)
	if err != nil {
		return err
	}
	sc := smart.NewClient(a.RestConfig, a.ServiceCatalogConfig, clientset, scClient)
	scheme, err := apitypes.FullScheme(a.ServiceCatalogConfig != nil)
	if err != nil {
		return err
	}

	// Stager will perform ordered, graceful shutdown
	stgr := stager.New()
	defer stgr.Shutdown()

	// Multi store
	multiStore := store.NewMulti()

	// Informers
	bundleInf := client.BundleInformer(bundleClient.SmithV1(), a.Namespace, a.ResyncPeriod)
	multiStore.AddInformer(smith_v1.BundleGVK, bundleInf)

	informerFactory := crdInformers.NewSharedInformerFactory(crdClient, a.ResyncPeriod)
	crdInf := informerFactory.Apiextensions().V1beta1().CustomResourceDefinitions().Informer()
	crdStore, err := store.NewCrd(crdInf)
	if err != nil {
		return err
	}
	crdGVK := apiext_v1b1.SchemeGroupVersion.WithKind("CustomResourceDefinition")
	multiStore.AddInformer(crdGVK, crdInf)
	stage := stgr.NextStage()
	stage.StartWithChannel(crdInf.Run) // Must be after store.AddInformer()

	// We must wait for crdInf to populate its cache to avoid reading from an empty cache
	// in Ready Checker and in EnsureCrdExists().
	log.Printf("Waiting for %s informer to sync", crdGVK)
	if !cache.WaitForCacheSync(ctx.Done(), crdInf.HasSynced) {
		return ctx.Err()
	}

	// Ensure CRD Bundle exists
	crdLister := informerFactory.Apiextensions().V1beta1().CustomResourceDefinitions().Lister()
	crdCreationBackoff := wait.Backoff{
		Duration: 1 * time.Second,
		Factor:   1.2,
		Jitter:   0.1,
		Steps:    7,
	}
	crd := resources.BundleCrd()
	err = wait.ExponentialBackoff(crdCreationBackoff, func() (bool /*done*/, error) {
		if errEnsure := resources.EnsureCrdExistsAndIsEstablished(ctx, scheme, crdClient, crdLister, crd); errEnsure != nil {
			// TODO be smarter about what is retried
			if errEnsure == context.Canceled || errEnsure == context.DeadlineExceeded {
				return true, errEnsure
			}
			log.Printf("Failed to create CRD %s: %v", crd.Name, errEnsure)
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		return err
	}

	// Controller
	bs, err := store.NewBundle(bundleInf, multiStore)
	if err != nil {
		return err
	}
	resourceInfs := apitypes.ResourceInformers(clientset, scClient, a.Namespace, a.ResyncPeriod, !a.DisablePodPreset)
	cntrlr := a.controller(bundleInf, crdInf, bundleClient.SmithV1(), bs, crdStore, sc, scheme, multiStore, resourceInfs, plugins)

	infs := make([]cache.InformerSynced, 0, len(resourceInfs)+1)
	// Add all informers to Multi store and start them
	for gvk, inf := range resourceInfs {
		multiStore.AddInformer(gvk, inf)
		stage.StartWithChannel(inf.Run) // Must be after AddInformer()
		infs = append(infs, inf.HasSynced)
	}
	stage.StartWithChannel(bundleInf.Run)
	infs = append(infs, bundleInf.HasSynced)
	log.Print("Waiting for informers to sync")
	if !cache.WaitForCacheSync(ctx.Done(), infs...) {
		return ctx.Err()
	}

	cntrlr.Run(ctx)
	return ctx.Err()
}

func (a *App) controller(bundleInf, crdInf cache.SharedIndexInformer, bundleClient smithClient_v1.BundlesGetter, bundleStore controller.BundleStore, crdStore readychecker.CrdStore,
	sc smith.SmartClient, scheme *runtime.Scheme, cStore controller.Store, resourceInfs map[schema.GroupVersionKind]cache.SharedIndexInformer, plugins map[smith_v1.PluginName]plugin.Plugin) *controller.BundleController {

	// Ready Checker
	readyTypes := []map[schema.GroupKind]readychecker.IsObjectReady{ready_types.MainKnownTypes}
	if a.ServiceCatalogConfig != nil {
		readyTypes = append(readyTypes, ready_types.ServiceCatalogKnownTypes)
	}
	rc := readychecker.New(crdStore, readyTypes...)

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

	return controller.New(bundleInf, crdInf, bundleClient, bundleStore, sc, rc, cStore, specCheck, queue, a.Workers, a.ResyncPeriod, resourceInfs, a.Namespace, plugins, scheme)
}

func (a *App) loadPlugins() (map[smith_v1.PluginName]plugin.Plugin, error) {
	plugs := make(map[smith_v1.PluginName]plugin.Plugin, len(a.Plugins))
	for _, p := range a.Plugins {
		plug, err := p()
		if err != nil {
			return nil, errors.Wrapf(err, "failed to instantiate plugin %T", p)
		}
		description := plug.Describe()
		if _, ok := plugs[description.Name]; ok {
			return nil, errors.Wrapf(err, "plugins with same name found %q", description.Name)
		}
		plugs[description.Name] = plug
	}
	return plugs, nil
}

// CancelOnInterrupt calls f when os.Interrupt or SIGTERM is received.
// It ignores subsequent interrupts on purpose - program should exit correctly after the first signal.
func CancelOnInterrupt(ctx context.Context, f context.CancelFunc) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		select {
		case <-ctx.Done():
		case <-c:
			f()
		}
	}()
}

func (a *App) ParseFlags(flagset *flag.FlagSet, arguments []string) error {
	flagset.BoolVar(&a.DisablePodPreset, "disable-pod-preset", false, "Disable PodPreset support")
	scDisable := flagset.Bool("disable-service-catalog", false, "Disable Service Catalog support")
	scUrl := flagset.String("service-catalog-url", "", "Service Catalog API server URL")
	scInsecure := flagset.Bool("service-catalog-insecure", false, "Disable TLS validation for Service Catalog")
	flagset.DurationVar(&a.ResyncPeriod, "resync-period", defaultResyncPeriod, "Resync period for informers")
	flagset.StringVar(&a.Namespace, "namespace", meta_v1.NamespaceAll, "Namespace to use. All namespaces are used if empty string or omitted")
	pprofAddr := flag.String("pprof-address", "", "Address for pprof to listen on")
	if err := flagset.Parse(arguments); err != nil {
		return err
	}

	config, err := client.ConfigFromEnv()
	if err != nil {
		config, err = rest.InClusterConfig()
		if err != nil {
			return err
		}
	}
	config.UserAgent = "smith"
	a.RestConfig = config
	if !*scDisable {
		scConfig := *config // shallow copy
		if *scInsecure {
			scConfig.TLSClientConfig.Insecure = true
			scConfig.TLSClientConfig.CAFile = ""
			scConfig.TLSClientConfig.CAData = nil
		}
		if *scUrl != "" {
			scConfig.Host = *scUrl
		}
		a.ServiceCatalogConfig = &scConfig
	}

	if pprofAddr != nil && *pprofAddr != "" {
		go func() {
			log.Fatalf("pprof server failed: %v", http.ListenAndServe(*pprofAddr, nil))
		}()
	}
	return nil
}
