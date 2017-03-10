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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
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

	tmplClient, tmplScheme, err := resources.GetTemplateTprClient(a.RestConfig)
	if err != nil {
		return err
	}

	clients := dynamic.NewClientPool(a.RestConfig, nil, dynamic.LegacyAPIPathResolverFunc)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// 1. TPR Informer
	tprInf := tprInformer(ctx, clientset)

	// We must wait for tprInf to populate its cache to avoid reading from an empty cache
	// in Ready Checker.
	if !cache.WaitForCacheSync(ctx.Done(), tprInf.HasSynced) {
		return errors.New("wait for TPR Informer was cancelled")
	}

	// 2. Ready Checker

	rc := readychecker.New(&tprStore{
		store: tprInf.GetStore(),
	})

	// 3. Processor

	tp := processor.New(ctx, tmplClient, clients, rc, tmplScheme)
	defer tp.Join() // await termination
	defer cancel()  // cancel ctx to signal done to processor (and everything else)

	// 4. Ensure ThirdPartyResource TEMPLATE exists
	err = retryUntilSuccessOrDone(ctx, func() error {
		return ensureResourceExists(clientset)
	}, func(e error) bool {
		// TODO be smarter about what is retried
		log.Printf("Failed to create resource %s: %v", smith.TemplateResourceName, e)
		return false
	})
	if err != nil {
		return err
	}

	// 5. Watch Templates
	tmplInf, err := watchTemplates(ctx, tmplClient, tmplScheme, tp)
	if err != nil {
		return err
	}

	// We must wait for tmplInf to populate its cache to avoid reading from an empty cache
	// in case of resource-generated events.
	if !cache.WaitForCacheSync(ctx.Done(), tmplInf.HasSynced) {
		return errors.New("wait for Template Informer was cancelled")
	}

	sl := templateStore{store: tmplInf.GetStore()}
	reh := &resourceEventHandler{
		processor: tp,
		name2tmpl: sl.Get,
	}

	// 6. TODO watch supported built-in resource types for events.

	// 7. Watch Third Party Resources to add watches for supported ones

	tprInf.AddEventHandler(newTprEventHandler(ctx, reh, clients))

	<-ctx.Done()
	return ctx.Err()
}

func ensureResourceExists(clientset kubernetes.Interface) error {
	log.Printf("Creating ThirdPartyResource %s", smith.TemplateResourceName)
	tpr := &extensions.ThirdPartyResource{
		ObjectMeta: metav1.ObjectMeta{
			Name: smith.TemplateResourceName,
		},
		Description: "Smith resource manager",
		Versions: []extensions.APIVersion{
			{Name: smith.TemplateResourceVersion},
		},
	}
	res, err := clientset.ExtensionsV1beta1().ThirdPartyResources().Create(tpr)
	if err != nil {
		if !kerrors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create %s ThirdPartyResource: %v", smith.TemplateResourceName, err)
		}
		// TODO handle conflicts and update properly
		//log.Printf("ThirdPartyResource %s already exists, updating", smith.TemplateResourceName)
		//_, err = clientset.ExtensionsV1beta1().ThirdPartyResources().Update(tpr)
		//if err != nil {
		//	return fmt.Errorf("failed to update %s ThirdPartyResource: %v",  smith.TemplateResourceName, err)
		//}
	} else {
		log.Printf("ThirdPartyResource %s created: %+v", smith.TemplateResourceName, res)
		// TODO It takes a while for k8s to add a new rest endpoint. Polling?
		time.Sleep(10 * time.Second)
	}
	return nil
}

func tprInformer(ctx context.Context, clientset kubernetes.Interface) cache.SharedInformer {
	tprClient := clientset.ExtensionsV1beta1().ThirdPartyResources()
	tprInf := cache.NewSharedInformer(&cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			return tprClient.List(options)
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			return tprClient.Watch(options)
		},
	}, &extensions.ThirdPartyResource{}, 0)

	go tprInf.Run(ctx.Done())

	return tprInf
}

func watchTemplates(ctx context.Context, tmplClient cache.Getter, tmplScheme *runtime.Scheme, processor Processor) (cache.SharedInformer, error) {
	parameterCodec := runtime.NewParameterCodec(tmplScheme)

	// Cannot use cache.NewListWatchFromClient() because it uses global api.ParameterCodec which uses global
	// api.Scheme which does not know about Smith group/version.
	// cache.NewListWatchFromClient(templateClient, smith.TemplateResourcePath, apiv1.NamespaceAll, fields.Everything())
	tmplInf := cache.NewSharedInformer(&cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			return tmplClient.Get().
				Resource(smith.TemplateResourcePath).
				VersionedParams(&options, parameterCodec).
				Do().
				Get()
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			return tmplClient.Get().
				Prefix("watch").
				Resource(smith.TemplateResourcePath).
				VersionedParams(&options, parameterCodec).
				Watch()
		},
	}, &smith.Template{}, 0)

	tmplInf.AddEventHandler(&templateEventHandler{processor: processor})

	go tmplInf.Run(ctx.Done())

	return tmplInf, nil
}
