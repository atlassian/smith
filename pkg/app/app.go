package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/atlassian/smith"
	"github.com/atlassian/smith/pkg/processor"
	"github.com/atlassian/smith/pkg/resources"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api"
	"k8s.io/client-go/pkg/api/errors"
	"k8s.io/client-go/pkg/api/unversioned"
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

	templateClient, err := resources.GetTemplateTprClient(a.RestConfig)
	if err != nil {
		return err
	}

	clients := dynamic.NewClientPool(a.RestConfig, nil, dynamic.LegacyAPIPathResolverFunc)

	var cancel context.CancelFunc
	ctx, cancel = context.WithCancel(ctx)
	defer cancel()

	tp := processor.New(ctx, templateClient, clients, &StatusReadyChecker{})
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
	a.watchTemplates(ctx, templateClient, tp)

	<-ctx.Done()
	return ctx.Err()
}

func (a *App) ensureResourceExists(ctx context.Context, clientset *kubernetes.Clientset) error {
	log.Printf("Creating ThirdPartyResource %s", smith.TemplateResourceName)
	res, err := clientset.ExtensionsV1beta1().ThirdPartyResources().Create(&extensions.ThirdPartyResource{
		TypeMeta: unversioned.TypeMeta{
			Kind: "ThirdPartyResource",
			//APIVersion: smith.ThirdPartyResourceGroupVersion,
		},
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

func (a *App) watchThirdPartyResources(ctx context.Context, clientset *kubernetes.Clientset, clients dynamic.ClientPool, processor Processor) {
	tprClient := clientset.ExtensionsV1beta1().ThirdPartyResources()
	tprInf := cache.NewSharedInformer(&cache.ListWatch{
		ListFunc: func(options api.ListOptions) (runtime.Object, error) {
			var opts apiv1.ListOptions
			if err := apiv1.Convert_api_ListOptions_To_v1_ListOptions(&options, &opts, nil); err != nil {
				return nil, err
			}
			return tprClient.List(opts)
		},
		WatchFunc: func(options api.ListOptions) (watch.Interface, error) {
			var opts apiv1.ListOptions
			if err := apiv1.Convert_api_ListOptions_To_v1_ListOptions(&options, &opts, nil); err != nil {
				return nil, err
			}
			return tprClient.Watch(opts)
		},
	}, &extensions.ThirdPartyResource{}, 0)

	tprInf.AddEventHandler(newTprEventHandler(ctx, processor, clients))

	go tprInf.Run(ctx.Done())
}

func (a *App) watchTemplates(ctx context.Context, templateClient *rest.RESTClient, processor Processor) {
	templateInf := cache.NewSharedInformer(&cache.ListWatch{
		ListFunc: func(options api.ListOptions) (runtime.Object, error) {
			list := &smith.TemplateList{}
			err := templateClient.Get().
				Resource(smith.TemplateResourcePath).
				//VersionedParams(&options, runtime.NewParameterCodec(api.Scheme)).
				Do().
				Into(list)
			if err != nil {
				return nil, err
			}
			return list, nil
		},
		WatchFunc: func(options api.ListOptions) (watch.Interface, error) {
			r, err := templateClient.Get().
				Prefix("watch").
				Resource(smith.TemplateResourcePath).
				//VersionedParams(&options, runtime.NewParameterCodec(api.Scheme)).
				Stream()
			if err != nil {
				return nil, err
			}
			return watch.NewStreamWatcher(&templateDecoder{
				decoder: json.NewDecoder(r),
				close:   r.Close,
			}), nil
		},
	}, &smith.Template{}, 0)

	templateInf.AddEventHandler(newTemplateEventHandler(processor))

	go templateInf.Run(ctx.Done())
}
