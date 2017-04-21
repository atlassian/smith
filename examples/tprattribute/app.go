package tprattribute

import (
	"context"
	"sync"
	"time"

	"github.com/atlassian/smith"
	"github.com/atlassian/smith/pkg/resources"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	apiv1 "k8s.io/client-go/pkg/api/v1"
	extensions "k8s.io/client-go/pkg/apis/extensions/v1beta1"
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
	scheme.AddUnversionedTypes(apiv1.SchemeGroupVersion, &metav1.Status{})
	AddToScheme(scheme)
	sClient, err := GetSleeperTprClient(a.RestConfig, scheme)
	if err != nil {
		return err
	}

	store := resources.NewStore(scheme.DeepCopy)

	var wgStore sync.WaitGroup
	defer wgStore.Wait() // await store termination

	ctxStore, cancelStore := context.WithCancel(context.Background())
	defer cancelStore() // signal store to stop
	wgStore.Add(1)
	go store.Run(ctxStore, wgStore.Done)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	informerFactory := informers.NewSharedInformerFactory(clientset, ResyncPeriod)
	tprInf := informerFactory.Extensions().V1beta1().ThirdPartyResources().Informer()
	store.AddInformer(smith.TprGVK, tprInf)
	informerFactory.Start(ctx.Done()) // Must be after store.AddInformer()

	// 1. Ensure ThirdPartyResource Sleeper exists
	tpr := &extensions.ThirdPartyResource{
		ObjectMeta: metav1.ObjectMeta{
			Name: SleeperResourceName,
			Annotations: map[string]string{
				smith.TprFieldPathAnnotation:  "status.state",
				smith.TprFieldValueAnnotation: string(Awake),
			},
		},
		Description: "Sleeper TPR Informer example",
		Versions: []extensions.APIVersion{
			{Name: SleeperResourceVersion},
		},
	}
	err = resources.EnsureTprExists(ctx, clientset, store, tpr)
	if err != nil {
		return err
	}

	// 2. Create an Informer for Sleeper objects
	err = sleeperInformer(ctx, sClient)
	if err != nil {
		return err
	}

	// 3. Wait for a signal to stop
	<-ctx.Done()
	return ctx.Err()
}

func sleeperInformer(ctx context.Context, sClient *rest.RESTClient) error {
	tmplInf := cache.NewSharedInformer(
		cache.NewListWatchFromClient(sClient, SleeperResourcePath, metav1.NamespaceAll, fields.Everything()),
		&Sleeper{}, 0)

	seh := &SleeperEventHandler{
		ctx:    ctx,
		client: sClient,
	}

	tmplInf.AddEventHandler(seh)

	// Run the Informer concurrently
	go tmplInf.Run(ctx.Done())

	return nil
}
