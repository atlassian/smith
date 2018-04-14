package app

import (
	"flag"
	"time"

	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/smith/pkg/cleanup"
	clean_types "github.com/atlassian/smith/pkg/cleanup/types"
	"github.com/atlassian/smith/pkg/client"
	"github.com/atlassian/smith/pkg/controller"
	"github.com/atlassian/smith/pkg/controller/bundlec"
	"github.com/atlassian/smith/pkg/plugin"
	"github.com/atlassian/smith/pkg/readychecker"
	ready_types "github.com/atlassian/smith/pkg/readychecker/types"
	"github.com/atlassian/smith/pkg/speccheck"
	"github.com/atlassian/smith/pkg/store"
	"github.com/atlassian/smith/pkg/util/logz"
	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	scClientset "github.com/kubernetes-incubator/service-catalog/pkg/client/clientset_generated/clientset"
	sc_v1b1inf "github.com/kubernetes-incubator/service-catalog/pkg/client/informers_generated/externalversions/servicecatalog/v1beta1"
	"github.com/pkg/errors"
	apps_v1 "k8s.io/api/apps/v1"
	core_v1 "k8s.io/api/core/v1"
	ext_v1b1 "k8s.io/api/extensions/v1beta1"
	apiext_v1b1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	apiext_v1b1inf "k8s.io/apiextensions-apiserver/pkg/client/informers/externalversions/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	apps_v1inf "k8s.io/client-go/informers/apps/v1"
	core_v1inf "k8s.io/client-go/informers/core/v1"
	ext_v1b1inf "k8s.io/client-go/informers/extensions/v1beta1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

type BundleControllerConstructor struct {
	Plugins               []plugin.NewFunc
	ServiceCatalogSupport bool
}

func (c *BundleControllerConstructor) AddFlags(flagset *flag.FlagSet) {
	flagset.BoolVar(&c.ServiceCatalogSupport, "bundle-service-catalog", true, "Service Catalog support in Bundle controller. Enabled by default.")
}

func (c *BundleControllerConstructor) New(config *controller.Config, cctx *controller.Context) (controller.Interface, error) {
	// Plugins
	pluginContainers, err := c.loadPlugins()
	if err != nil {
		return nil, err
	}
	for pluginName := range pluginContainers {
		config.Logger.Sugar().Infof("Loaded plugin: %q", pluginName)
	}
	scheme, err := FullScheme(c.ServiceCatalogSupport)
	if err != nil {
		return nil, err
	}
	// Informers
	bundleInf, err := cctx.SmithInformer(config, smith_v1.BundleGVK, client.BundleInformer)
	if err != nil {
		return nil, err
	}
	crdInf, err := cctx.ApiExtensionsInformer(config,
		apiext_v1b1.SchemeGroupVersion.WithKind("CustomResourceDefinition"),
		apiext_v1b1inf.NewCustomResourceDefinitionInformer)
	if err != nil {
		return nil, err
	}
	crdStore, err := store.NewCrd(crdInf)
	if err != nil {
		return nil, err
	}

	var catalog *store.Catalog
	if c.ServiceCatalogSupport {
		serviceClassInf, err := cctx.SvcCatClusterInformer(config,
			sc_v1b1.SchemeGroupVersion.WithKind("ClusterServiceClass"),
			sc_v1b1inf.NewClusterServiceClassInformer)
		if err != nil {
			return nil, err
		}
		servicePlanInf, err := cctx.SvcCatClusterInformer(config,
			sc_v1b1.SchemeGroupVersion.WithKind("ClusterServicePlan"),
			sc_v1b1inf.NewClusterServicePlanInformer)
		if err != nil {
			return nil, err
		}
		catalog, err = store.NewCatalog(serviceClassInf, servicePlanInf)
		if err != nil {
			return nil, err
		}
	}

	// Ready Checker
	readyTypes := []map[schema.GroupKind]readychecker.IsObjectReady{ready_types.MainKnownTypes}
	if c.ServiceCatalogSupport {
		readyTypes = append(readyTypes, ready_types.ServiceCatalogKnownTypes)
	}
	rc := readychecker.New(crdStore, readyTypes...)

	// Object cleanup
	cleanupTypes := []map[schema.GroupKind]cleanup.SpecCleanup{clean_types.MainKnownTypes}
	if c.ServiceCatalogSupport {
		cleanupTypes = append(cleanupTypes, clean_types.ServiceCatalogKnownTypes)
	}
	oc := cleanup.New(cleanupTypes...)

	// Spec check
	specCheck := &speccheck.SpecCheck{
		Logger:  config.Logger,
		Cleaner: oc,
	}

	// Multi store
	multiStore := store.NewMulti()

	bs, err := store.NewBundle(bundleInf, multiStore, pluginContainers)
	if err != nil {
		return nil, err
	}

	// Add resource informers to Multi store (not ServiceClass/Plan informers, ...)
	resourceInfs, err := c.resourceInformers(config, cctx)
	if err != nil {
		return nil, err
	}
	resourceInfs[apiext_v1b1.SchemeGroupVersion.WithKind("CustomResourceDefinition")] = crdInf
	resourceInfs[smith_v1.BundleGVK] = bundleInf
	for gvk, inf := range resourceInfs {
		if err = multiStore.AddInformer(gvk, inf); err != nil {
			return nil, errors.Errorf("failed to add informer for %s", gvk)
		}
	}

	// Controller
	cntrlr := &bundlec.Controller{
		Logger:           config.Logger,
		BundleClient:     config.SmithClient.SmithV1(),
		BundleStore:      bs,
		SmartClient:      config.SmartClient,
		Rc:               rc,
		Store:            multiStore,
		SpecCheck:        specCheck,
		WorkQueue:        cctx.WorkQueue,
		CrdResyncPeriod:  config.ResyncPeriod,
		Namespace:        config.Namespace,
		PluginContainers: pluginContainers,
		Scheme:           scheme,
		Catalog:          catalog,
	}
	cntrlr.Prepare(crdInf, resourceInfs)

	return cntrlr, nil
}

func (c *BundleControllerConstructor) Describe() controller.Descriptor {
	return controller.Descriptor{
		Gvk:          smith_v1.BundleGVK,
		ZapNameField: logz.BundleName,
	}
}

func (c *BundleControllerConstructor) loadPlugins() (map[smith_v1.PluginName]plugin.PluginContainer, error) {
	pluginContainers := make(map[smith_v1.PluginName]plugin.PluginContainer, len(c.Plugins))
	for _, p := range c.Plugins {
		pluginContainer, err := plugin.NewPluginContainer(p)
		if err != nil {
			return nil, err
		}
		description := pluginContainer.Plugin.Describe()
		if _, ok := pluginContainers[description.Name]; ok {
			return nil, errors.Errorf("plugins with same name found %q", description.Name)
		}
		pluginContainers[description.Name] = pluginContainer
	}
	return pluginContainers, nil
}

func (c *BundleControllerConstructor) resourceInformers(config *controller.Config, cctx *controller.Context) (map[schema.GroupVersionKind]cache.SharedIndexInformer, error) {
	coreInfs := map[schema.GroupVersionKind]func(kubernetes.Interface, string, time.Duration, cache.Indexers) cache.SharedIndexInformer{
		// Core API types
		ext_v1b1.SchemeGroupVersion.WithKind("Ingress"):       ext_v1b1inf.NewIngressInformer,
		core_v1.SchemeGroupVersion.WithKind("Service"):        core_v1inf.NewServiceInformer,
		core_v1.SchemeGroupVersion.WithKind("ConfigMap"):      core_v1inf.NewConfigMapInformer,
		core_v1.SchemeGroupVersion.WithKind("Secret"):         core_v1inf.NewSecretInformer,
		core_v1.SchemeGroupVersion.WithKind("ServiceAccount"): core_v1inf.NewServiceAccountInformer,
		apps_v1.SchemeGroupVersion.WithKind("Deployment"):     apps_v1inf.NewDeploymentInformer,
	}
	infs := make(map[schema.GroupVersionKind]cache.SharedIndexInformer, len(coreInfs)+2)
	for gvk, coreInf := range coreInfs {
		inf, err := cctx.MainInformer(config, gvk, coreInf)
		if err != nil {
			return nil, err
		}
		infs[gvk] = inf
	}

	// Service Catalog types
	if c.ServiceCatalogSupport {
		scInfs := map[schema.GroupVersionKind]func(scClientset.Interface, string, time.Duration, cache.Indexers) cache.SharedIndexInformer{
			// Service Catalog types
			sc_v1b1.SchemeGroupVersion.WithKind("ServiceBinding"):  sc_v1b1inf.NewServiceBindingInformer,
			sc_v1b1.SchemeGroupVersion.WithKind("ServiceInstance"): sc_v1b1inf.NewServiceInstanceInformer,
		}
		for gvk, scInf := range scInfs {
			inf, err := cctx.SvcCatInformer(config, gvk, scInf)
			if err != nil {
				return nil, err
			}
			infs[gvk] = inf
		}
	}

	return infs, nil
}

func FullScheme(serviceCatalog bool) (*runtime.Scheme, error) {
	scheme := runtime.NewScheme()
	var sb runtime.SchemeBuilder
	sb.Register(smith_v1.SchemeBuilder...)
	sb.Register(ext_v1b1.SchemeBuilder...)
	sb.Register(core_v1.SchemeBuilder...)
	sb.Register(apps_v1.SchemeBuilder...)
	sb.Register(apiext_v1b1.SchemeBuilder...)
	if serviceCatalog {
		sb.Register(sc_v1b1.SchemeBuilder...)
	}
	if err := sb.AddToScheme(scheme); err != nil {
		return nil, err
	}
	return scheme, nil
}
