package app

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/smith/pkg/cleanup"
	clean_types "github.com/atlassian/smith/pkg/cleanup/types"
	"github.com/atlassian/smith/pkg/client"
	smithClientset "github.com/atlassian/smith/pkg/client/clientset_generated/clientset"
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
	"github.com/ghodss/yaml"
	scClientset "github.com/kubernetes-incubator/service-catalog/pkg/client/clientset_generated/clientset"
	"github.com/pkg/errors"
	apiext_v1b1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	crdClientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apiext_v1b1inf "k8s.io/apiextensions-apiserver/pkg/client/informers/externalversions/apiextensions/v1beta1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
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

	// Informers
	bundleInf := client.BundleInformer(bundleClient.SmithV1(), a.Namespace, a.ResyncPeriod)
	crdInf := apiext_v1b1inf.NewCustomResourceDefinitionInformer(crdClient, a.ResyncPeriod, cache.Indexers{})
	crdStore, err := store.NewCrd(crdInf)
	if err != nil {
		return err
	}

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

	// Multi store
	multiStore := store.NewMulti()

	bs, err := store.NewBundle(bundleInf, multiStore, plugins)
	if err != nil {
		return err
	}
	resourceInfs := apitypes.ResourceInformers(clientset, scClient, a.Namespace, a.ResyncPeriod, !a.DisablePodPreset)

	// Controller
	cntrlr := controller.BundleController{
		BundleInf:       bundleInf,
		BundleClient:    bundleClient.SmithV1(),
		BundleStore:     bs,
		SmartClient:     sc,
		Rc:              rc,
		Store:           multiStore,
		SpecCheck:       specCheck,
		Queue:           workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "bundle"),
		Workers:         a.Workers,
		CrdResyncPeriod: a.ResyncPeriod,
		Namespace:       a.Namespace,
		Plugins:         plugins,
		Scheme:          scheme,
	}
	cntrlr.Prepare(ctx, crdInf, resourceInfs)

	resourceInfs[apiext_v1b1.SchemeGroupVersion.WithKind("CustomResourceDefinition")] = crdInf
	resourceInfs[smith_v1.BundleGVK] = bundleInf

	// Stager will perform ordered, graceful shutdown
	stgr := stager.New()
	defer stgr.Shutdown()
	stage := stgr.NextStage()

	infs := make([]cache.InformerSynced, 0, len(resourceInfs))
	// Add all informers to Multi store and start them
	for gvk, inf := range resourceInfs {
		multiStore.AddInformer(gvk, inf)
		stage.StartWithChannel(inf.Run) // Must be after AddInformer()
		infs = append(infs, inf.HasSynced)
	}
	log.Print("Waiting for informers to sync")
	if !cache.WaitForCacheSync(ctx.Done(), infs...) {
		return ctx.Err()
	}

	cntrlr.Run(ctx)
	return ctx.Err()
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

func NewFromFlags(flagset *flag.FlagSet, arguments []string) (*App, error) {
	a := App{}
	flagset.BoolVar(&a.DisablePodPreset, "disable-pod-preset", false, "Disable PodPreset support")
	scDisable := flagset.Bool("disable-service-catalog", false, "Disable Service Catalog support")
	scUrl := flagset.String("service-catalog-url", "", "Service Catalog API server URL")
	scInsecure := flagset.Bool("service-catalog-insecure", false, "Disable TLS validation for Service Catalog")
	flagset.DurationVar(&a.ResyncPeriod, "resync-period", defaultResyncPeriod, "Resync period for informers")
	flagset.IntVar(&a.Workers, "workers", 2, "Number of workers that handle events from informers")
	flagset.StringVar(&a.Namespace, "namespace", meta_v1.NamespaceAll, "Namespace to use. All namespaces are used if empty string or omitted")
	pprofAddr := flag.String("pprof-address", "", "Address for pprof to listen on")
	printBundleSchema := flag.String("print-bundle-schema", "", "Print Bundle schema and exit (specify format: json or yaml)")
	if err := flagset.Parse(arguments); err != nil {
		return nil, err
	}

	switch *printBundleSchema {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		err := enc.Encode(resources.BundleCrd().Spec.Validation.OpenAPIV3Schema)
		if err != nil {
			return nil, errors.Wrap(err, "failed to marshal Bundle schema")
		}
		return nil, context.Canceled
	case "yaml":
		data, err := yaml.Marshal(resources.BundleCrd().Spec.Validation.OpenAPIV3Schema)
		if err != nil {
			return nil, errors.Wrap(err, "failed to marshal Bundle schema")
		}
		_, err = os.Stdout.Write(data)
		if err != nil {
			return nil, errors.Wrap(err, "failed write schema YAML to stdout")
		}
		return nil, context.Canceled
	case "":
		// flag not set, continue to execute the normal flow
	default:
		return nil, errors.Errorf("unsupported schema output format %q", *printBundleSchema)
	}

	config, err := client.ConfigFromEnv()
	if err != nil {
		config, err = rest.InClusterConfig()
		if err != nil {
			return nil, err
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
	return &a, nil
}
