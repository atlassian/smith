package app

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/atlassian/smith"
	"github.com/atlassian/smith/pkg/resources"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	extensions "k8s.io/client-go/pkg/apis/extensions/v1beta1"
	"k8s.io/client-go/tools/cache"
)

type informerStore interface {
	AddInformer(schema.GroupVersionKind, cache.SharedIndexInformer)
	RemoveInformer(schema.GroupVersionKind) bool
}

type watchState struct {
	cancel  context.CancelFunc
	version extensions.APIVersion
}

// tprEventHandler handles events for objects with Kind: ThirdPartyResource.
// For each object a new informer is started to watch for events.
type tprEventHandler struct {
	ctx          context.Context
	handler      cache.ResourceEventHandler
	clients      dynamic.ClientPool
	store        informerStore
	resyncPeriod time.Duration
	mx           sync.Mutex
	watchers     map[string]map[string]watchState // TPR name -> TPR version -> state
}

func newTprEventHandler(ctx context.Context, handler cache.ResourceEventHandler, clients dynamic.ClientPool,
	store informerStore, resyncPeriod time.Duration) *tprEventHandler {
	return &tprEventHandler{
		ctx:          ctx,
		handler:      handler,
		clients:      clients,
		store:        store,
		resyncPeriod: resyncPeriod,
		watchers:     make(map[string]map[string]watchState),
	}
}

func (h *tprEventHandler) OnAdd(obj interface{}) {
	tpr := obj.(*extensions.ThirdPartyResource)
	if tpr.Name == smith.BundleResourceName {
		return
	}
	log.Printf("[TPREH] Handling OnAdd for TPR %s", tpr.Name)
	h.mx.Lock()
	defer h.mx.Unlock()
	h.watchVersions(tpr.Name, tpr.Versions...)

	// TODO rebuild all bundles containing resources of this type
}

func (h *tprEventHandler) OnUpdate(oldObj, newObj interface{}) {
	newTpr := newObj.(*extensions.ThirdPartyResource)
	if newTpr.Name == smith.BundleResourceName {
		return
	}
	newVersions := versionsMap(newTpr)

	var added []extensions.APIVersion
	var removed []extensions.APIVersion

	h.mx.Lock()
	defer h.mx.Unlock()

	tprWatch := h.watchers[newTpr.Name]

	// Comparing to existing state, not to oldObj for better resiliency to errors
	for versionName, state := range tprWatch {
		if _, ok := newVersions[versionName]; !ok {
			removed = append(removed, state.version)
		}
	}

	for _, v := range newVersions {
		state, ok := tprWatch[v.Name]
		if ok {
			// If some fields are added in the future and this update changes them, we want to update our state
			state.version = v
		} else {
			added = append(added, v)
		}
	}

	h.unwatchVersions(newTpr.Name, removed...)
	h.watchVersions(newTpr.Name, added...)

	// TODO rebuild all bundles containing resources of this type
}

func versionsMap(tpr *extensions.ThirdPartyResource) map[string]extensions.APIVersion {
	v := make(map[string]extensions.APIVersion, len(tpr.Versions))
	for _, ver := range tpr.Versions {
		v[ver.Name] = ver
	}
	return v
}

func (h *tprEventHandler) OnDelete(obj interface{}) {
	tpr := obj.(*extensions.ThirdPartyResource)
	h.mx.Lock()
	defer h.mx.Unlock()

	// Removing all watched versions for this TPR
	tprWatch := h.watchers[tpr.Name]
	versions := make([]extensions.APIVersion, 0, len(tprWatch))

	for _, state := range tprWatch {
		versions = append(versions, state.version)
	}

	h.unwatchVersions(tpr.Name, versions...)
	// TODO rebuild all bundles containing resources of this type
}

func (h *tprEventHandler) watchVersions(tprName string, versions ...extensions.APIVersion) {
	if len(versions) == 0 {
		return
	}
	tprWatch := h.watchers[tprName]
	if tprWatch == nil {
		tprWatch = make(map[string]watchState)
		h.watchers[tprName] = tprWatch
	}
	path, gk := resources.SplitTprName(tprName)
	for _, version := range versions {
		log.Printf("[TPREH] Configuring watch for TPR %s version %s", tprName, version.Name)
		gvk := gk.WithVersion(version.Name)
		dc, err := h.clients.ClientForGroupVersionKind(gvk)
		if err != nil {
			log.Printf("[TPREH] Failed to instantiate client for TPR %s of version %s: %v", tprName, version.Name, err)
			continue
		}
		res := dc.Resource(&metav1.APIResource{
			Name: path,
			Kind: gk.Kind,
		}, metav1.NamespaceAll)
		tprInf := cache.NewSharedIndexInformer(&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return res.List(options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return res.Watch(options)
			},
		}, &unstructured.Unstructured{}, h.resyncPeriod, cache.Indexers{})

		tprInf.AddEventHandler(h.handler)

		ctx, cancel := context.WithCancel(h.ctx)

		tprWatch[version.Name] = watchState{cancel: cancel, version: version}

		h.store.AddInformer(gvk, tprInf)

		go tprInf.Run(ctx.Done())
	}
}

func (h *tprEventHandler) unwatchVersions(tprName string, versions ...extensions.APIVersion) {
	tprWatch := h.watchers[tprName]
	if tprWatch == nil {
		// Nothing to do. This can happen if there was an error adding a watch
		return
	}
	for _, version := range versions {
		if ws, ok := tprWatch[version.Name]; ok {
			log.Printf("[TPREH] Removing watch for TPR %s version %s", tprName, version.Name)
			delete(tprWatch, version.Name)
			ws.cancel()
		}
	}
	if len(tprWatch) == 0 {
		delete(h.watchers, tprName)
	}
}
