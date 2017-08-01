package store

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/atlassian/smith"
	"github.com/atlassian/smith/pkg/resources"

	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"
)

const (
	ByNamespaceAndBundleNameIndex = "NamespaceBundleNameIndex"
)

var (
	ErrInformerRemoved = errors.New("informer was removed")
)

type AwaitCallback func(runtime.Object, error)

type AwaitCondition func(runtime.Object) bool

type awaitRequest struct {
	gvk      schema.GroupVersionKind
	name     types.NamespacedName
	cond     AwaitCondition
	callback AwaitCallback
}

type awaitResult struct {
	obj runtime.Object
	err error
}

type addInformerRequest struct {
	wg       *sync.WaitGroup
	gvk      schema.GroupVersionKind
	informer cache.SharedIndexInformer
}

type getInformersRequest struct {
	result chan<- map[schema.GroupVersionKind]cache.SharedIndexInformer
}

type removeInformerRequest struct {
	gvk    schema.GroupVersionKind
	result chan<- bool
}

type informerEvent struct {
	gvk schema.GroupVersionKind
	obj runtime.Object
}

type Multi struct {
	deepCopy    smith.DeepCopy
	informers   map[schema.GroupVersionKind]cache.SharedIndexInformer
	gvk2request map[schema.GroupVersionKind]map[types.NamespacedName]map[*awaitRequest]struct{} // GVK -> namespace/name -> requests

	awaitRequests          chan *awaitRequest // must be a pointer because it is used as a key in map
	informerEvents         chan informerEvent
	cancellations          chan *awaitRequest // must be a pointer because it is used as a key in map
	addInformerRequests    chan addInformerRequest
	getInformersRequests   chan getInformersRequest
	removeInformerRequests chan removeInformerRequest
}

func NewMulti(deepCopy smith.DeepCopy) *Multi {
	return &Multi{
		deepCopy:               deepCopy,
		informers:              make(map[schema.GroupVersionKind]cache.SharedIndexInformer),
		gvk2request:            make(map[schema.GroupVersionKind]map[types.NamespacedName]map[*awaitRequest]struct{}),
		awaitRequests:          make(chan *awaitRequest),
		informerEvents:         make(chan informerEvent),
		cancellations:          make(chan *awaitRequest),
		addInformerRequests:    make(chan addInformerRequest),
		getInformersRequests:   make(chan getInformersRequest),
		removeInformerRequests: make(chan removeInformerRequest),
	}
}

func (s *Multi) Run(ctx context.Context) {
	// Multi must not be used after Run exited
	defer close(s.awaitRequests)
	defer close(s.cancellations)
	defer close(s.addInformerRequests)
	defer close(s.getInformersRequests)
	defer close(s.removeInformerRequests)
	for {
		select {
		case <-ctx.Done():
			// unblock all awaiting callers
			for _, m := range s.gvk2request {
				removeAllCallbacks(m)
			}
			return
		case ar := <-s.awaitRequests:
			s.handleAwaitRequest(ar)
		case ie := <-s.informerEvents:
			s.handleEvent(ie.gvk, ie.obj)
		case ar := <-s.cancellations:
			s.handleCancellation(ar)
		case air := <-s.addInformerRequests:
			s.handleAddInformer(ctx, air.gvk, air.informer, air.wg)
		case gir := <-s.getInformersRequests:
			s.handleGetInformers(gir.result)
		case rir := <-s.removeInformerRequests:
			s.handleRemoveInformer(rir.gvk, rir.result)
		}
	}
}

func (s *Multi) handleAwaitRequest(ar *awaitRequest) {
	informer, ok := s.informers[ar.gvk]
	if !ok {
		ar.callback(nil, fmt.Errorf("no informer for %v is registered", ar.gvk))
		return
	}
	obj, exists, err := s.getFromIndexer(informer.GetIndexer(), ar.gvk, ar.name.Namespace, ar.name.Name)
	if err != nil || (exists && ar.cond(obj)) {
		ar.callback(obj, err)
		return
	}
	// Object is not in the store (yet) OR does not satisfy the condition
	m := s.gvk2request[ar.gvk]
	if m == nil {
		m = make(map[types.NamespacedName]map[*awaitRequest]struct{})
		s.gvk2request[ar.gvk] = m
	}
	n := m[ar.name]
	if n == nil {
		n = make(map[*awaitRequest]struct{})
		m[ar.name] = n
	}
	n[ar] = struct{}{}
}

func (s *Multi) handleEvent(gvk schema.GroupVersionKind, obj runtime.Object) {
	metaObj := obj.(meta_v1.Object)
	name := types.NamespacedName{Namespace: metaObj.GetNamespace(), Name: metaObj.GetName()}
	m := s.gvk2request[gvk]
	n := m[name]
	for ar := range n {
		// Must deep copy and set GVK before checking cond() to be consistent with how handleAwaitRequest() works.
		// And it makes sense in any case.
		o, err := s.deepCopy(obj) // Each callback gets its own copy
		if err != nil {
			delete(n, ar)
			ar.callback(nil, fmt.Errorf("failed to deep copy %T: %v", obj, err))
			continue
		}
		ro := o.(runtime.Object)
		ro.GetObjectKind().SetGroupVersionKind(gvk) // Objects from type-specific informers don't have GVK set
		if ar.cond(ro) {
			delete(n, ar)
			ar.callback(ro, nil)
		}
	}
	if len(n) == 0 {
		delete(m, name)
		if len(m) == 0 {
			delete(s.gvk2request, gvk)
		}
	}
}

func (s *Multi) handleCancellation(ar *awaitRequest) {
	m := s.gvk2request[ar.gvk]
	n := m[ar.name]
	delete(n, ar)
	if len(n) == 0 {
		delete(m, ar.name)
		if len(m) == 0 {
			delete(s.gvk2request, ar.gvk)
		}
	}
}

func (s *Multi) handleAddInformer(ctx context.Context, gvk schema.GroupVersionKind, informer cache.SharedIndexInformer, wg *sync.WaitGroup) {
	defer wg.Done()
	if _, ok := s.informers[gvk]; ok {
		// This is a programming error hence panic
		panic(fmt.Errorf("Informer for %v is already registered", gvk))
	}
	err := informer.AddIndexers(cache.Indexers{
		ByNamespaceAndBundleNameIndex: byNamespaceAndBundleNameIndex,
	})
	if err != nil {
		// Must never happen in our case
		panic(err)
	}
	informer.AddEventHandler(&listener{ctx: ctx, gvk: gvk, events: s.informerEvents})
	s.informers[gvk] = informer
}

func (s *Multi) handleGetInformers(result chan<- map[schema.GroupVersionKind]cache.SharedIndexInformer) {
	informers := make(map[schema.GroupVersionKind]cache.SharedIndexInformer, len(s.informers))
	for gvk, inf := range s.informers {
		informers[gvk] = inf
	}
	result <- informers
}

func (s *Multi) handleRemoveInformer(gvk schema.GroupVersionKind, result chan<- bool) {
	_, ok := s.informers[gvk]
	if ok {
		delete(s.informers, gvk)
		removeAllCallbacks(s.gvk2request[gvk])
		delete(s.gvk2request, gvk)
	}
	result <- ok
}

func removeAllCallbacks(m map[types.NamespacedName]map[*awaitRequest]struct{}) {
	for _, n := range m {
		for ar := range n {
			ar.callback(nil, ErrInformerRemoved)
		}
	}
}

// AddInformer adds an Informer to the store.
// Can only be called with a not yet started informer. Otherwise bad things will happen.
func (s *Multi) AddInformer(gvk schema.GroupVersionKind, informer cache.SharedIndexInformer) {
	air := addInformerRequest{
		wg:       &sync.WaitGroup{},
		gvk:      gvk,
		informer: informer,
	}
	air.wg.Add(1)
	s.addInformerRequests <- air
	air.wg.Wait() // Wait for the informer to be processed
}

func (s *Multi) RemoveInformer(gvk schema.GroupVersionKind) bool {
	result := make(chan bool)
	rir := removeInformerRequest{
		gvk:    gvk,
		result: result,
	}
	s.removeInformerRequests <- rir
	return <-result
}

// GetInformers gets all registered Informers.
func (s *Multi) GetInformers() map[schema.GroupVersionKind]cache.SharedIndexInformer {
	result := make(chan map[schema.GroupVersionKind]cache.SharedIndexInformer)
	s.getInformersRequests <- getInformersRequest{result: result}
	return <-result
}

// AwaitObject looks up object of specified GVK in the specified namespace by name.
// This is a variant of Get method that blocks until the object is available or context signals "done".
// A deep copy of the object is returned so it is safe to modify it.
func (s *Multi) AwaitObject(ctx context.Context, gvk schema.GroupVersionKind, namespace, name string) (runtime.Object, error) {
	return s.AwaitObjectCondition(ctx, gvk, namespace, name, func(obj runtime.Object) bool {
		return true
	})
}

// AwaitObjectCondition looks up object of specified GVK in the specified namespace by name.
// This is a variant of AwaitObject method that blocks until the object is available and satisfies the condition or context signals "done".
// A deep copy of the object is returned so it is safe to modify it.
func (s *Multi) AwaitObjectCondition(ctx context.Context, gvk schema.GroupVersionKind, namespace, name string, cond AwaitCondition) (runtime.Object, error) {
	result := make(chan awaitResult)
	ar := &awaitRequest{
		gvk:  gvk,
		name: types.NamespacedName{Namespace: namespace, Name: name},
		cond: cond,
		callback: func(obj runtime.Object, err error) {
			result <- awaitResult{obj: obj, err: err}
		},
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case s.awaitRequests <- ar:
	}
	select {
	case <-ctx.Done():
	case res := <-result:
		return res.obj, res.err
	}
	select { // if ctx is done either send the cancellation or receive the result
	case s.cancellations <- ar:
		return nil, ctx.Err()
	case res := <-result:
		return res.obj, res.err
	}
}

// Get looks up object of specified GVK in the specified namespace by name.
// A deep copy of the object is returned so it is safe to modify it.
func (s *Multi) Get(gvk schema.GroupVersionKind, namespace, name string) (obj runtime.Object, exists bool, e error) {
	informer := s.GetInformers()[gvk]
	if informer == nil {
		return nil, false, fmt.Errorf("no informer for %v is registered", gvk)
	}
	return s.getFromIndexer(informer.GetIndexer(), gvk, namespace, name)
}

func (s *Multi) getFromIndexer(indexer cache.Indexer, gvk schema.GroupVersionKind, namespace, name string) (runtime.Object, bool /*exists */, error) {
	obj, exists, err := indexer.GetByKey(ByNamespaceAndNameIndexKey(namespace, name))
	if err != nil || !exists {
		return nil, exists, err
	}
	objCopy, err := s.deepCopy(obj)
	if err != nil {
		return nil, false, fmt.Errorf("failed to deep copy %T: %v", obj, err)
	}
	ro := objCopy.(runtime.Object)
	ro.GetObjectKind().SetGroupVersionKind(gvk) // Objects from type-specific informers don't have GVK set
	return ro, true, nil
}

func (s *Multi) GetObjectsForBundle(namespace, bundleName string) ([]runtime.Object, error) {
	var result []runtime.Object
	indexKey := ByNamespaceAndBundleNameIndexKey(namespace, bundleName)
	for gvk, inf := range s.GetInformers() {
		objs, err := inf.GetIndexer().ByIndex(ByNamespaceAndBundleNameIndex, indexKey)
		if err != nil {
			return nil, fmt.Errorf("failed to get objects for bundle from %s informer: %v", gvk, err)
		}
		for _, obj := range objs {
			o, err := s.deepCopy(obj)
			if err != nil {
				return nil, fmt.Errorf("failed to deep copy %T: %v", obj, err)
			}
			ro := o.(runtime.Object)
			ro.GetObjectKind().SetGroupVersionKind(gvk) // Objects from type-specific informers don't have GVK set
			result = append(result, ro)
		}
	}
	return result, nil
}

type listener struct {
	ctx    context.Context
	gvk    schema.GroupVersionKind
	events chan<- informerEvent
}

func (l *listener) OnAdd(obj interface{}) {
	l.handle(obj)
}

func (l *listener) OnUpdate(oldObj, newObj interface{}) {
	l.handle(newObj)
}

func (l *listener) OnDelete(obj interface{}) {
}

func (l *listener) handle(obj interface{}) {
	select {
	case <-l.ctx.Done():
	case l.events <- informerEvent{gvk: l.gvk, obj: obj.(runtime.Object)}:
	}
}

func byNamespaceAndBundleNameIndex(obj interface{}) ([]string, error) {
	if key, ok := obj.(cache.ExplicitKey); ok {
		return []string{string(key)}, nil
	}
	m := obj.(meta_v1.Object)
	ref := resources.GetControllerOf(m)
	if ref != nil && ref.APIVersion == smith.BundleResourceGroupVersion && ref.Kind == smith.BundleResourceKind {
		return []string{ByNamespaceAndBundleNameIndexKey(m.GetNamespace(), ref.Name)}, nil
	}
	return nil, nil
}

func ByNamespaceAndBundleNameIndexKey(namespace, bundleName string) string {
	return namespace + "|" + bundleName // Different separator to avoid clashes with ByNamespaceAndNameIndexKey
}

func ByNamespaceAndNameIndexKey(namespace, name string) string {
	if namespace == meta_v1.NamespaceNone {
		return name
	}
	return namespace + "/" + name
}
