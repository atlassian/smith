package app

import (
	"context"
	"fmt"
	"log"

	"github.com/atlassian/smith"
	"github.com/atlassian/smith/pkg/processor"
	"github.com/atlassian/smith/pkg/resources"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api/errors"
	apiv1 "k8s.io/client-go/pkg/api/v1"
	extensions "k8s.io/client-go/pkg/apis/extensions/v1beta1"
	"k8s.io/client-go/pkg/runtime"
	"k8s.io/client-go/pkg/watch"
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

	tp := processor.New(ctx, tmplClient, clients, &StatusReadyChecker{})
	defer tp.Join() // await termination
	defer cancel()  // cancel ctx to signal done to processor (and everything else)

	// 1. Ensure ThirdPartyResource TEMPLATE exists
	err = retryUntilSuccessOrDone(ctx, func() error {
		return a.ensureResourceExists(ctx, clientset)
	}, func(e error) bool {
		// TODO be smarter about what is retried
		log.Printf("Failed to create resource %s: %v", smith.TemplateResourceName, e)
		return false
	})
	if err != nil {
		return err
	}

	// 2. TODO watch supported built-in resource types for events.

	// 3. Watch Third Party Resources to add watches for supported ones
	a.watchThirdPartyResources(ctx, clientset, clients, tp)

	// 4. Watch Templates
	a.watchTemplates(ctx, tmplClient, tmplScheme, tp)

	<-ctx.Done()
	return ctx.Err()
}

func (a *App) ensureResourceExists(ctx context.Context, clientset kubernetes.Interface) error {
	log.Printf("Creating ThirdPartyResource %s", smith.TemplateResourceName)
	res, err := clientset.ExtensionsV1beta1().ThirdPartyResources().Create(&extensions.ThirdPartyResource{
		ObjectMeta: apiv1.ObjectMeta{
			Name: smith.TemplateResourceName,
		},
		Description: "Smith resource manager",
		Versions: []extensions.APIVersion{
			{Name: smith.TemplateResourceVersion},
		},
	})
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create ThirdPartyResource: %v", err)
		}
		log.Printf("ThirdPartyResource %s already exists", smith.TemplateResourceName)
	} else {
		log.Printf("ThirdPartyResource %s created: %+v", smith.TemplateResourceName, res)
	}
	return nil
}

func (a *App) watchThirdPartyResources(ctx context.Context, clientset kubernetes.Interface, clients dynamic.ClientPool, processor Processor) {
	tprClient := clientset.ExtensionsV1beta1().ThirdPartyResources()
	tprInf := cache.NewSharedInformer(&cache.ListWatch{
		ListFunc: func(options apiv1.ListOptions) (runtime.Object, error) {
			return tprClient.List(options)
		},
		WatchFunc: func(options apiv1.ListOptions) (watch.Interface, error) {
			return tprClient.Watch(options)
		},
	}, &extensions.ThirdPartyResource{}, 0)

	tprInf.AddEventHandler(newTprEventHandler(ctx, processor, clients))

	go tprInf.Run(ctx.Done())
}

func (a *App) watchTemplates(ctx context.Context, tmplClient cache.Getter, tmplScheme *runtime.Scheme, processor Processor) {
	parameterCodec := runtime.NewParameterCodec(tmplScheme)

	// Cannot use cache.NewListWatchFromClient() because it uses global api.ParameterCodec which uses global
	// api.Scheme which does not know about Smith group/version.
	// cache.NewListWatchFromClient(templateClient, smith.TemplateResourcePath, apiv1.NamespaceAll, fields.Everything())
	templateInf := cache.NewSharedInformer(&cache.ListWatch{
		ListFunc: func(options apiv1.ListOptions) (runtime.Object, error) {
			return tmplClient.Get().
				Resource(smith.TemplateResourcePath).
				VersionedParams(&options, parameterCodec).
				Do().
				Get()
		},
		WatchFunc: func(options apiv1.ListOptions) (watch.Interface, error) {
			return tmplClient.Get().
				Prefix("watch").
				Resource(smith.TemplateResourcePath).
				VersionedParams(&options, parameterCodec).
				Watch()
		},
	}, &smith.Template{}, 0)

	templateInf.AddEventHandler(newTemplateEventHandler(processor))

	go templateInf.Run(ctx.Done())
}
