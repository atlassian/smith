package templatecontroller

import (
	"context"
	"sync"
	"time"

	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	smithClient_v1 "github.com/atlassian/smith/pkg/client/clientset_generated/clientset/typed/smith/v1"
	"github.com/atlassian/smith/pkg/util/logz"

	"github.com/ash2k/stager/wait"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

const (
	// Work queue deduplicates scheduled keys. This is the period it waits for duplicate keys before letting the work
	// to be dequeued.
	workDeduplicationPeriod = 50 * time.Millisecond
)

type SpecCheck interface {
	CompareActualVsSpec(spec, actual runtime.Object) (updatedSpec *unstructured.Unstructured, match bool, err error)
}

type ReadyChecker interface {
	IsReady(*unstructured.Unstructured) (isReady, retriableError bool, e error)
}

type SmartClient interface {
	ForGVK(gvk schema.GroupVersionKind, namespace string) (dynamic.ResourceInterface, error)
}

type Store interface {
	Get(gvk schema.GroupVersionKind, namespace, name string) (obj runtime.Object, exists bool, err error)
	ObjectsControlledBy(namespace string, uid types.UID) ([]runtime.Object, error)
	AddInformer(schema.GroupVersionKind, cache.SharedIndexInformer)
	RemoveInformer(schema.GroupVersionKind) bool
}

type TemplateController struct {
	// wg.Wait() is called from Run() and first wg.Add() may be called concurrently from CRD listener
	// to start an Informer. This is a data race. This mutex is used to ensure ordering.
	// See https://github.com/atlassian/smith/issues/156
	// See https://github.com/golang/go/blob/fbc8973a6bc88b50509ea738f475b36ef756bf90/src/sync/waitgroup.go#L123-L126
	wgLock   sync.Mutex
	wg       wait.Group
	stopping bool

	Logger *zap.Logger

	TemplateRenderInf    cache.SharedIndexInformer
	TemplateInf          cache.SharedIndexInformer
	TemplateRenderClient smithClient_v1.BundlesGetter
	SmartClient          SmartClient
	Store                Store
	SpecCheck            SpecCheck

	// TemplateRender objects that need to be synced.
	Queue   workqueue.RateLimitingInterface
	Workers int

	Scheme *runtime.Scheme
}

// Prepare prepares the controller to be run.
// ctx must be the same context as the one passed to Run() method.
func (tc *TemplateController) Prepare() {
	tc.TemplateRenderInf.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    tc.onTemplateRenderAdd,
		UpdateFunc: tc.onTemplateRenderUpdate,
		DeleteFunc: tc.onTemplateRenderDelete,
	})
	// TODO add AddFunc etc. for Template. Need to also to index the TemplateRenders
	// by Template so we can enqueue...
	// Not to mention enqueuing for the generated resource mutations. Probably should share that.
}

// Run begins watching and syncing.
// All informers must be synced before this method is invoked.
func (tc *TemplateController) Run(ctx context.Context) {
	defer tc.wg.Wait()
	defer func() {
		tc.wgLock.Lock()
		defer tc.wgLock.Unlock()
		tc.stopping = true
	}()
	defer tc.Queue.ShutDown()

	tc.Logger.Info("Starting Template controller")
	defer tc.Logger.Info("Shutting down Template controller")

	for i := 0; i < tc.Workers; i++ {
		tc.wg.Start(tc.worker)
	}

	<-ctx.Done()
}

func (tc *TemplateController) enqueue(templateRender *smith_v1.TemplateRender) {
	key, err := cache.MetaNamespaceKeyFunc(templateRender)
	if err != nil {
		tc.Logger.Error("Couldn't get key for TemplateRender", logz.Namespace(templateRender), logz.TemplateRender(templateRender), zap.Error(err))
		return
	}
	tc.enqueueKey(key)
}

func (tc *TemplateController) enqueueKey(key string) {
	tc.Queue.AddAfter(key, workDeduplicationPeriod)
}
