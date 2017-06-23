package tprattribute

import (
	"context"
	"errors"
	"time"

	"github.com/atlassian/smith"
	"github.com/atlassian/smith/pkg/resources"
	"github.com/atlassian/smith/pkg/store"

	"github.com/ash2k/stager"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	api_v1 "k8s.io/client-go/pkg/api/v1"
	ext_v1b1 "k8s.io/client-go/pkg/apis/extensions/v1beta1"
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
	clientset, err := kubernetes.NewForConfig(a.RestConfig)
	if err != nil {
		return err
	}

	scheme := runtime.NewScheme()
	scheme.AddUnversionedTypes(api_v1.SchemeGroupVersion, &meta_v1.Status{})
	AddToScheme(scheme)
	sClient, err := GetSleeperTprClient(a.RestConfig, scheme)
	if err != nil {
		return err
	}

	stgr := stager.New()
	defer stgr.Shutdown()

	stage := stgr.NextStage()
	multiStore := store.NewMulti(scheme.DeepCopy)
	stage.StartWithContext(multiStore.Run)

	informerFactory := informers.NewSharedInformerFactory(clientset, ResyncPeriod)
	tprInf := informerFactory.Extensions().V1beta1().ThirdPartyResources().Informer()
	multiStore.AddInformer(ext_v1b1.SchemeGroupVersion.WithKind("ThirdPartyResource"), tprInf)
	stage = stgr.NextStage()
	stage.StartWithChannel(tprInf.Run) // Must be after multiStore.AddInformer()

	// 1. Ensure ThirdPartyResource Sleeper exists

	// We must wait for tprInf to populate its cache to avoid reading from an empty cache
	// in resources.EnsureTprExists().
	if !cache.WaitForCacheSync(ctx.Done(), tprInf.HasSynced) {
		return errors.New("wait for TPR Informer was cancelled")
	}

	if err = resources.EnsureTprExists(ctx, clientset, multiStore, SleeperTpr()); err != nil {
		return err
	}

	// 2. Create an Informer for Sleeper objects
	sleeperInformer := sleeperInformer(ctx, sClient, scheme.DeepCopy)
	stage.StartWithChannel(sleeperInformer.Run)

	// 3. Wait for a signal to stop
	<-ctx.Done()
	return ctx.Err()
}

func sleeperInformer(ctx context.Context, sClient *rest.RESTClient, deepCopy smith.DeepCopy) cache.SharedInformer {
	sleeperInf := cache.NewSharedInformer(
		cache.NewListWatchFromClient(sClient, SleeperResourcePath, meta_v1.NamespaceAll, fields.Everything()),
		&Sleeper{}, 0)

	seh := &SleeperEventHandler{
		ctx:      ctx,
		client:   sClient,
		deepCopy: deepCopy,
	}

	sleeperInf.AddEventHandler(seh)

	return sleeperInf
}
