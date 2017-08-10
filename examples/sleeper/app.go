package sleeper

import (
	"context"
	"errors"
	"time"

	"github.com/atlassian/smith"
	"github.com/atlassian/smith/pkg/resources"
	"github.com/atlassian/smith/pkg/store"

	"github.com/ash2k/stager"
	apiext_v1b1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	crdClientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	crdInformers "k8s.io/apiextensions-apiserver/pkg/client/informers/externalversions"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	api_v1 "k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

const (
	ResyncPeriod = 20 * time.Minute
)

type App struct {
	RestConfig *rest.Config
}

func (a *App) Run(ctx context.Context) error {
	scheme := runtime.NewScheme()
	scheme.AddUnversionedTypes(api_v1.SchemeGroupVersion, &meta_v1.Status{})
	AddToScheme(scheme)
	if err := apiext_v1b1.SchemeBuilder.AddToScheme(scheme); err != nil {
		return err
	}

	sClient, err := GetSleeperClient(a.RestConfig, scheme)
	if err != nil {
		return err
	}
	crdClient, err := crdClientset.NewForConfig(a.RestConfig)
	if err != nil {
		return err
	}

	stgr := stager.New()
	defer stgr.Shutdown()

	multiStore := store.NewMulti(scheme.DeepCopy)

	informerFactory := crdInformers.NewSharedInformerFactory(crdClient, ResyncPeriod)
	crdInf := informerFactory.Apiextensions().V1beta1().CustomResourceDefinitions().Informer()
	multiStore.AddInformer(apiext_v1b1.SchemeGroupVersion.WithKind("CustomResourceDefinition"), crdInf)
	stage := stgr.NextStage()
	stage.StartWithChannel(crdInf.Run) // Must be after multiStore.AddInformer()

	// 1. Ensure CRD Sleeper exists

	// We must wait for crdInf to populate its cache to avoid reading from an empty cache
	// in resources.EnsureCrdExists().
	if !cache.WaitForCacheSync(ctx.Done(), crdInf.HasSynced) {
		return errors.New("wait for CRD informer was cancelled")
	}

	crdLister := informerFactory.Apiextensions().V1beta1().CustomResourceDefinitions().Lister()
	if err = resources.EnsureCrdExistsAndIsEstablished(ctx, scheme, crdClient, crdLister, SleeperCrd()); err != nil {
		return err
	}

	// 2. Create an Informer for Sleeper objects
	sleeperInformer := sleeperInformer(ctx, sClient, scheme.DeepCopy)
	stage.StartWithChannel(sleeperInformer.Run)

	// 3. Wait for a signal to stop
	<-ctx.Done()
	return ctx.Err()
}

func sleeperInformer(ctx context.Context, sClient rest.Interface, deepCopy smith.DeepCopy) cache.SharedInformer {
	sleeperInf := cache.NewSharedInformer(
		cache.NewListWatchFromClient(sClient, SleeperResourcePlural, meta_v1.NamespaceAll, fields.Everything()),
		&Sleeper{}, ResyncPeriod)

	seh := &SleeperEventHandler{
		ctx:      ctx,
		client:   sClient,
		deepCopy: deepCopy,
	}

	sleeperInf.AddEventHandler(seh)

	return sleeperInf
}
