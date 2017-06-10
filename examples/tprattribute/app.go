package tprattribute

import (
	"context"
	"errors"
	"time"

	"github.com/atlassian/smith"
	"github.com/atlassian/smith/pkg/resources"
	"github.com/atlassian/smith/pkg/util/wait"

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
	ResyncPeriod = 1 * time.Minute
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

	store := resources.NewStore(scheme.DeepCopy)

	var wgStore wait.Group
	defer wgStore.Wait() // await store termination

	ctxStore, cancelStore := context.WithCancel(context.Background())
	defer cancelStore() // signal store to stop
	wgStore.StartWithContext(ctxStore, store.Run)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	informerFactory := informers.NewSharedInformerFactory(clientset, ResyncPeriod)
	tprInf := informerFactory.Extensions().V1beta1().ThirdPartyResources().Informer()
	store.AddInformer(ext_v1b1.SchemeGroupVersion.WithKind("ThirdPartyResource"), tprInf)
	informerFactory.Start(ctx.Done()) // Must be after store.AddInformer()

	// 1. Ensure ThirdPartyResource Sleeper exists

	// We must wait for tprInf to populate its cache to avoid reading from an empty cache
	// in resources.EnsureTprExists().
	if !cache.WaitForCacheSync(ctx.Done(), tprInf.HasSynced) {
		return errors.New("wait for TPR Informer was cancelled")
	}

	tpr := &ext_v1b1.ThirdPartyResource{
		ObjectMeta: meta_v1.ObjectMeta{
			Name: SleeperResourceName,
			Annotations: map[string]string{
				smith.TprFieldPathAnnotation:  "status.state",
				smith.TprFieldValueAnnotation: string(Awake),
			},
		},
		Description: "Sleeper TPR Informer example",
		Versions: []ext_v1b1.APIVersion{
			{Name: SleeperResourceVersion},
		},
	}
	err = resources.EnsureTprExists(ctx, clientset, store, tpr)
	if err != nil {
		return err
	}

	// 2. Create an Informer for Sleeper objects
	err = sleeperInformer(ctx, sClient, scheme.DeepCopy)
	if err != nil {
		return err
	}

	// 3. Wait for a signal to stop
	<-ctx.Done()
	return ctx.Err()
}

func sleeperInformer(ctx context.Context, sClient *rest.RESTClient, deepCopy smith.DeepCopy) error {
	tmplInf := cache.NewSharedInformer(
		cache.NewListWatchFromClient(sClient, SleeperResourcePath, meta_v1.NamespaceAll, fields.Everything()),
		&Sleeper{}, 0)

	seh := &SleeperEventHandler{
		ctx:      ctx,
		client:   sClient,
		deepCopy: deepCopy,
	}

	tmplInf.AddEventHandler(seh)

	// Run the Informer concurrently
	go tmplInf.Run(ctx.Done())

	return nil
}
