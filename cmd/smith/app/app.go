package app

import (
	"context"
	"flag"
	"fmt"
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
	"github.com/atlassian/smith/pkg/resources/apitypes"
	"github.com/atlassian/smith/pkg/speccheck"
	"github.com/atlassian/smith/pkg/store"

	"github.com/ash2k/stager"
	scClientset "github.com/kubernetes-incubator/service-catalog/pkg/client/clientset_generated/clientset"
	"github.com/pkg/errors"
	core_v1 "k8s.io/api/core/v1"
	apiext_v1b1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	crdClientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apiext_v1b1inf "k8s.io/apiextensions-apiserver/pkg/client/informers/externalversions/apiextensions/v1beta1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	core_v1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
)

const (
	defaultResyncPeriod  = 20 * time.Minute
	defaultLeaseDuration = 15 * time.Second
	defaultRenewDeadline = 10 * time.Second
	defaultRetryPeriod   = 2 * time.Second
)

// See kubernetes/kubernetes/pkg/apis/componentconfig/types.go LeaderElectionConfiguration
// for leader election configuration description.
type LeaderElectionConfig struct {
	LeaderElect        bool
	LeaseDuration      time.Duration
	RenewDeadline      time.Duration
	RetryPeriod        time.Duration
	ConfigMapNamespace string
	ConfigMapName      string
}

type App struct {
	RestConfig            *rest.Config
	ResyncPeriod          time.Duration
	Namespace             string
	Plugins               []plugin.NewFunc
	Workers               int
	PodPresetSupport      bool
	ServiceCatalogSupport bool
	LeaderElectionConfig  LeaderElectionConfig
}

func (a *App) Run(ctx context.Context) error {
	// Plugins
	pluginContainers, err := a.loadPlugins()
	if err != nil {
		return err
	}
	for pluginName := range pluginContainers {
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
	if a.ServiceCatalogSupport {
		scClient, err = scClientset.NewForConfig(a.RestConfig)
		if err != nil {
			return err
		}
	}
	crdClient, err := crdClientset.NewForConfig(a.RestConfig)
	if err != nil {
		return err
	}
	sc := smart.NewClient(a.RestConfig, clientset)
	scheme, err := apitypes.FullScheme(a.ServiceCatalogSupport)
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
	if a.ServiceCatalogSupport {
		readyTypes = append(readyTypes, ready_types.ServiceCatalogKnownTypes)
	}
	rc := readychecker.New(scheme, crdStore, readyTypes...)

	// Object cleanup
	cleanupTypes := []map[schema.GroupKind]cleanup.SpecCleanup{clean_types.MainKnownTypes}
	if a.ServiceCatalogSupport {
		cleanupTypes = append(cleanupTypes, clean_types.ServiceCatalogKnownTypes)
	}
	oc := cleanup.New(scheme, cleanupTypes...)

	// Spec check
	specCheck := &speccheck.SpecCheck{
		Scheme:  scheme,
		Cleaner: oc,
	}

	// Multi store
	multiStore := store.NewMulti()

	bs, err := store.NewBundle(bundleInf, multiStore, pluginContainers)
	if err != nil {
		return err
	}

	// Events
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(logInfo)
	eventBroadcaster.StartRecordingToSink(&core_v1client.EventSinkImpl{Interface: clientset.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(scheme, core_v1.EventSource{Component: "smith-controller"})

	// Leader election
	if a.LeaderElectionConfig.LeaderElect {
		log.Printf("Starting leader election in namespace %q", a.LeaderElectionConfig.ConfigMapNamespace)

		var startedLeading <-chan struct{}
		ctx, startedLeading, err = a.startLeaderElection(ctx, clientset.CoreV1(), recorder)
		if err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-startedLeading:
		}
	}

	resourceInfs := apitypes.ResourceInformers(clientset, scClient, a.Namespace, a.ResyncPeriod, a.PodPresetSupport)

	// Controller
	cntrlr := controller.BundleController{
		BundleInf:        bundleInf,
		BundleClient:     bundleClient.SmithV1(),
		BundleStore:      bs,
		SmartClient:      sc,
		Rc:               rc,
		Store:            multiStore,
		SpecCheck:        specCheck,
		Queue:            workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "bundle"),
		Workers:          a.Workers,
		CrdResyncPeriod:  a.ResyncPeriod,
		Namespace:        a.Namespace,
		PluginContainers: pluginContainers,
		Scheme:           scheme,
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

func (a *App) startLeaderElection(ctx context.Context, configMapsGetter core_v1client.ConfigMapsGetter, recorder record.EventRecorder) (context.Context, <-chan struct{}, error) {
	id, err := os.Hostname()
	if err != nil {
		return nil, nil, err
	}
	ctxRet, cancel := context.WithCancel(ctx)
	startedLeading := make(chan struct{})
	le, err := leaderelection.NewLeaderElector(leaderelection.LeaderElectionConfig{
		Lock: &resourcelock.ConfigMapLock{
			ConfigMapMeta: meta_v1.ObjectMeta{
				Namespace: a.LeaderElectionConfig.ConfigMapNamespace,
				Name:      a.LeaderElectionConfig.ConfigMapName,
			},
			Client: configMapsGetter,
			LockConfig: resourcelock.ResourceLockConfig{
				Identity:      id + "-smith",
				EventRecorder: recorder,
			},
		},
		LeaseDuration: a.LeaderElectionConfig.LeaseDuration,
		RenewDeadline: a.LeaderElectionConfig.RenewDeadline,
		RetryPeriod:   a.LeaderElectionConfig.RetryPeriod,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: func(stop <-chan struct{}) {
				log.Print("Started leading")
				close(startedLeading)
			},
			OnStoppedLeading: func() {
				log.Print("Leader status lost")
				cancel()
			},
		},
	})
	if err != nil {
		cancel()
		return nil, nil, err
	}
	go le.Run()
	return ctxRet, startedLeading, nil
}

func (a *App) loadPlugins() (map[smith_v1.PluginName]plugin.PluginContainer, error) {
	pluginContainers := make(map[smith_v1.PluginName]plugin.PluginContainer, len(a.Plugins))
	for _, p := range a.Plugins {
		pluginContainer, err := plugin.NewPluginContainer(p)
		if err != nil {
			return nil, err
		}
		description := pluginContainer.Plugin.Describe()
		if _, ok := pluginContainers[description.Name]; ok {
			return nil, errors.Wrapf(err, "plugins with same name found %q", description.Name)
		}
		pluginContainers[description.Name] = pluginContainer
	}
	return pluginContainers, nil
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
	flagset.BoolVar(&a.PodPresetSupport, "pod-preset", false, "PodPreset support. Disabled by default.")
	flagset.BoolVar(&a.ServiceCatalogSupport, "service-catalog", true, "Service Catalog support. Enabled by default.")
	flagset.DurationVar(&a.ResyncPeriod, "resync-period", defaultResyncPeriod, "Resync period for informers")
	flagset.IntVar(&a.Workers, "workers", 2, "Number of workers that handle events from informers")
	flagset.StringVar(&a.Namespace, "namespace", meta_v1.NamespaceAll, "Namespace to use. All namespaces are used if empty string or omitted")
	pprofAddr := flagset.String("pprof-address", "", "Address for pprof to listen on")

	// This flag is off by default only because leader election package says it is ALPHA API.
	flagset.BoolVar(&a.LeaderElectionConfig.LeaderElect, "leader-elect", false, ""+
		"Start a leader election client and gain leadership before "+
		"executing the main loop. Enable this when running replicated "+
		"components for high availability")
	flagset.DurationVar(&a.LeaderElectionConfig.LeaseDuration, "leader-elect-lease-duration", defaultLeaseDuration, ""+
		"The duration that non-leader candidates will wait after observing a leadership "+
		"renewal until attempting to acquire leadership of a led but unrenewed leader "+
		"slot. This is effectively the maximum duration that a leader can be stopped "+
		"before it is replaced by another candidate. This is only applicable if leader "+
		"election is enabled")
	flagset.DurationVar(&a.LeaderElectionConfig.RenewDeadline, "leader-elect-renew-deadline", defaultRenewDeadline, ""+
		"The interval between attempts by the acting master to renew a leadership slot "+
		"before it stops leading. This must be less than or equal to the lease duration. "+
		"This is only applicable if leader election is enabled")
	flagset.DurationVar(&a.LeaderElectionConfig.RetryPeriod, "leader-elect-retry-period", defaultRetryPeriod, ""+
		"The duration the clients should wait between attempting acquisition and renewal "+
		"of a leadership. This is only applicable if leader election is enabled")
	flagset.StringVar(&a.LeaderElectionConfig.ConfigMapNamespace, "leader-elect-configmap-namespace", meta_v1.NamespaceDefault,
		"Namespace to use for leader election ConfigMap. This is only applicable if leader election is enabled")
	flagset.StringVar(&a.LeaderElectionConfig.ConfigMapName, "leader-elect-configmap-name", "smith-leader-elect",
		"ConfigMap name to use for leader election. This is only applicable if leader election is enabled")
	configFileFrom := flagset.String("client-config-from", "in-cluster",
		"Source of REST client configuration. 'in-cluster' (default), 'environment' and 'file' are valid options.")
	configFileName := flagset.String("client-config-file-name", "",
		"Load REST client configuration from the specified Kubernetes config file. This is only applicable if --client-config-from=file is set.")
	configContext := flagset.String("client-config-context", "",
		"Context to use for REST client configuration. This is only applicable if --client-config-from=file is set.")

	if err := flagset.Parse(arguments); err != nil {
		return nil, err
	}

	config, err := client.LoadConfig(*configFileFrom, *configFileName, *configContext)
	if err != nil {
		return nil, err
	}

	config.UserAgent = "smith"
	a.RestConfig = config

	if *pprofAddr != "" {
		go func() {
			log.Fatalf("pprof server failed: %v", http.ListenAndServe(*pprofAddr, nil))
		}()
	}
	return &a, nil
}

func logInfo(format string, args ...interface{}) {
	log.Print(fmt.Sprintf(format, args...))
}
