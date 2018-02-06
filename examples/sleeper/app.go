package sleeper

import (
	"context"
	"time"

	sleeper_v1 "github.com/atlassian/smith/examples/sleeper/pkg/apis/sleeper/v1"
	"github.com/atlassian/smith/pkg/resources"
	"github.com/atlassian/smith/pkg/store"

	"github.com/ash2k/stager"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	core_v1 "k8s.io/api/core/v1"
	apiext_v1b1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	crdClientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apiext_v1b1inf "k8s.io/apiextensions-apiserver/pkg/client/informers/externalversions/apiextensions/v1beta1"
	apiext_v1b1list "k8s.io/apiextensions-apiserver/pkg/client/listers/apiextensions/v1beta1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

const (
	ResyncPeriod = 20 * time.Minute
)

type App struct {
	Logger     *zap.Logger
	RestConfig *rest.Config
	Namespace  string
}

func (a *App) Run(ctx context.Context) error {
	scheme := runtime.NewScheme()
	scheme.AddUnversionedTypes(core_v1.SchemeGroupVersion, &meta_v1.Status{})
	if err := sleeper_v1.AddToScheme(scheme); err != nil {
		return err
	}
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

	multiStore := store.NewMulti()

	crdInf := apiext_v1b1inf.NewCustomResourceDefinitionInformer(crdClient, ResyncPeriod, cache.Indexers{})
	multiStore.AddInformer(apiext_v1b1.SchemeGroupVersion.WithKind("CustomResourceDefinition"), crdInf)
	stage := stgr.NextStage()
	stage.StartWithChannel(crdInf.Run) // Must be after multiStore.AddInformer()

	// 1. Ensure CRD Sleeper exists

	// We must wait for crdInf to populate its cache to avoid reading from an empty cache
	// in resources.EnsureCrdExists().
	if !cache.WaitForCacheSync(ctx.Done(), crdInf.HasSynced) {
		return errors.New("wait for CRD informer was cancelled")
	}

	crdLister := apiext_v1b1list.NewCustomResourceDefinitionLister(crdInf.GetIndexer())
	if err = resources.EnsureCrdExistsAndIsEstablished(ctx, a.Logger, scheme, crdClient, crdLister, SleeperCrd()); err != nil {
		return err
	}

	// 2. Create an Informer for Sleeper objects
	sleeperInformer := a.sleeperInformer(ctx, sClient)

	// 3. Run until a signal to stop
	sleeperInformer.Run(ctx.Done())
	return ctx.Err()
}

func (a *App) sleeperInformer(ctx context.Context, sClient rest.Interface) cache.SharedInformer {
	sleeperInf := cache.NewSharedInformer(
		cache.NewListWatchFromClient(sClient, sleeper_v1.SleeperResourcePlural, a.Namespace, fields.Everything()),
		&sleeper_v1.Sleeper{}, ResyncPeriod)

	seh := &SleeperEventHandler{
		ctx:    ctx,
		logger: a.Logger,
		client: sClient,
	}

	sleeperInf.AddEventHandler(seh)

	return sleeperInf
}
