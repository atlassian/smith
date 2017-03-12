package tprattribute

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/atlassian/smith"

	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
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

	sClient, sScheme, err := GetSleeperTprClient(a.RestConfig)
	if err != nil {
		return err
	}

	// 1. Ensure ThirdPartyResource Sleeper exists
	err = ensureResourceExists(clientset)
	if err != nil {
		return err
	}

	// 2. Create an Informer for Sleeper objects
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	err = sleeperInformer(ctx, sClient, sScheme)
	if err != nil {
		return err
	}

	// 3. Wait for a signal to stop
	<-ctx.Done()
	return ctx.Err()
}

func ensureResourceExists(clientset kubernetes.Interface) error {
	log.Printf("Creating ThirdPartyResource %s", SleeperResourceName)
	tpr := &extensions.ThirdPartyResource{
		ObjectMeta: metav1.ObjectMeta{
			Name: SleeperResourceName,
			Annotations: map[string]string{
				smith.TprFieldPathAnnotation:  "status.state",
				smith.TprFieldValueAnnotation: string(AWAKE),
			},
		},
		Description: "Sleeper TPR Informer example",
		Versions: []extensions.APIVersion{
			{Name: SleeperResourceVersion},
		},
	}
	res, err := clientset.ExtensionsV1beta1().ThirdPartyResources().Create(tpr)
	if err != nil {
		if !kerrors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create %s ThirdPartyResource: %v", SleeperResourceName, err)
		}
		// TODO handle conflicts and update properly
		//log.Printf("ThirdPartyResource %s already exists, updating", SleeperResourceName)
		//_, err = clientset.ExtensionsV1beta1().ThirdPartyResources().Update(tpr)
		//if err != nil {
		//	return fmt.Errorf("failed to update %s ThirdPartyResource: %v", SleeperResourceName, err)
		//}
	} else {
		log.Printf("ThirdPartyResource %s created: %+v", SleeperResourceName, res)
		// TODO It takes a while for k8s to add a new rest endpoint. Polling?
		time.Sleep(10 * time.Second)
	}
	return nil
}

func sleeperInformer(ctx context.Context, sClient *rest.RESTClient, sScheme *runtime.Scheme) error {
	parameterCodec := runtime.NewParameterCodec(sScheme)

	tmplInf := cache.NewSharedInformer(&cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			return sClient.Get().
				Resource(SleeperResourcePath).
				VersionedParams(&options, parameterCodec).
				Context(ctx).
				Do().
				Get()
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			return sClient.Get().
				Prefix("watch").
				Resource(SleeperResourcePath).
				VersionedParams(&options, parameterCodec).
				Context(ctx).
				Watch()
		},
	}, &Sleeper{}, 0)

	seh := &SleeperEventHandler{
		ctx:    ctx,
		client: sClient,
	}

	tmplInf.AddEventHandler(seh)

	// Run the Informer concurrently
	go tmplInf.Run(ctx.Done())

	return nil
}
