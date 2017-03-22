package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/atlassian/smith"
	"github.com/atlassian/smith/pkg/processor"
	"github.com/atlassian/smith/pkg/readychecker"
	"github.com/atlassian/smith/pkg/resources"

	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	extensions "k8s.io/client-go/pkg/apis/extensions/v1beta1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

type App struct {
	RestConfig *rest.Config
}

func (a *App) Run(ctx context.Context) error {
	clientset, err := kubernetes.NewForConfig(a.RestConfig)
	if err != nil {
		return err
	}
	bundleScheme := resources.GetBundleScheme()
	bundleClient, err := resources.GetBundleTprClient(a.RestConfig, bundleScheme)
	if err != nil {
		return err
	}

	clients := dynamic.NewClientPool(a.RestConfig, nil, dynamic.LegacyAPIPathResolverFunc)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// 1. Informers

	informerFactory := informers.NewSharedInformerFactory(clientset, 1*time.Minute)
	tprInf := informerFactory.Extensions().V1beta1().ThirdPartyResources().Informer()
	deploymentInf := informerFactory.Extensions().V1beta1().Deployments().Informer()
	ingressInf := informerFactory.Extensions().V1beta1().Ingresses().Informer()
	serviceInf := informerFactory.Core().V1().Services().Informer()

	informerFactory.Start(ctx.Done())

	// We must wait for tprInf to populate its cache to avoid reading from an empty cache
	// in Ready Checker.
	if !cache.WaitForCacheSync(ctx.Done(), tprInf.HasSynced) {
		return errors.New("wait for TPR Informer was cancelled")
	}

	// 2. Ready Checker

	rc := &readychecker.ReadyChecker{
		Store: &tprStore{
			store: tprInf.GetStore(),
		},
	}

	// 3. Processor

	bp := processor.New(ctx, bundleClient, clients, rc, bundleScheme)
	defer bp.Join() // await termination
	defer cancel()  // cancel ctx to signal done to processor (and everything else)

	// 4. Ensure ThirdPartyResource TEMPLATE exists
	err = retryUntilSuccessOrDone(ctx, func() error {
		return ensureResourceExists(clientset)
	}, func(e error) bool {
		// TODO be smarter about what is retried
		log.Printf("Failed to create resource %s: %v", smith.BundleResourceName, e)
		return false
	})
	if err != nil {
		return err
	}

	// 5. Watch Bundles
	bundleInf, err := watchBundles(ctx, bundleClient, bundleScheme, bp)
	if err != nil {
		return err
	}

	// We must wait for bundleInf to populate its cache to avoid reading from an empty cache
	// in case of resource-generated events.
	if !cache.WaitForCacheSync(ctx.Done(), bundleInf.HasSynced) {
		return errors.New("wait for Bundle Informer was cancelled")
	}

	sl := bundleStore{
		store:  bundleInf.GetStore(),
		scheme: bundleScheme,
	}
	reh := &resourceEventHandler{
		processor:   bp,
		name2bundle: sl.Get,
	}

	// 6. Watch supported built-in resource types

	deploymentInf.AddEventHandler(reh)
	ingressInf.AddEventHandler(reh)
	serviceInf.AddEventHandler(reh)

	// 7. Watch Third Party Resources to add watches for supported ones

	tprInf.AddEventHandler(newTprEventHandler(ctx, reh, clients))

	<-ctx.Done()
	return ctx.Err()
}

func ensureResourceExists(clientset kubernetes.Interface) error {
	log.Printf("Creating ThirdPartyResource %s", smith.BundleResourceName)
	tpr := &extensions.ThirdPartyResource{
		ObjectMeta: metav1.ObjectMeta{
			Name: smith.BundleResourceName,
		},
		Description: "Smith resource manager",
		Versions: []extensions.APIVersion{
			{Name: smith.BundleResourceVersion},
		},
	}
	res, err := clientset.ExtensionsV1beta1().ThirdPartyResources().Create(tpr)
	if err != nil {
		if !kerrors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create %s ThirdPartyResource: %v", smith.BundleResourceName, err)
		}
		// TODO handle conflicts and update properly
		//log.Printf("ThirdPartyResource %s already exists, updating", smith.BundleResourceName)
		//_, err = clientset.ExtensionsV1beta1().ThirdPartyResources().Update(tpr)
		//if err != nil {
		//	return fmt.Errorf("failed to update %s ThirdPartyResource: %v",  smith.BundleResourceName, err)
		//}
	} else {
		log.Printf("ThirdPartyResource %s created: %+v", smith.BundleResourceName, res)
		// TODO It takes a while for k8s to add a new rest endpoint. Polling?
		time.Sleep(10 * time.Second)
	}
	return nil
}

func watchBundles(ctx context.Context, bundleClient cache.Getter, bundleScheme *runtime.Scheme, processor Processor) (cache.SharedInformer, error) {
	bundleInf := cache.NewSharedInformer(
		cache.NewListWatchFromClient(bundleClient, smith.BundleResourcePath, metav1.NamespaceAll, fields.Everything()),
		&smith.Bundle{},
		1*time.Minute)

	bundleInf.AddEventHandler(&bundleEventHandler{
		processor: processor,
		scheme:    bundleScheme,
	})

	go bundleInf.Run(ctx.Done())

	return bundleInf, nil
}
