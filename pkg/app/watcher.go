package app

import (
	"context"
	"fmt"
	"net/url"
	"sync"

	"github.com/ash2k/smith/pkg/client"
)

type watchState struct {
	cancel context.CancelFunc
}

// Watcher watches a modifiable set of resources, delivering their events.
// Not safe for concurrent use.
type Watcher struct {
	ctx      context.Context
	wg       sync.WaitGroup
	client   *client.ResourceClient
	events   chan<- interface{}
	watchers map[string]watchState
}

func NewWatcher(ctx context.Context, client *client.ResourceClient, events chan<- interface{}) *Watcher {
	return &Watcher{
		ctx:      ctx,
		client:   client,
		events:   events,
		watchers: make(map[string]watchState),
	}
}

func (w *Watcher) Join() {
	w.wg.Wait()
}

func (w *Watcher) Watch(groupVersion, namespace, resource, resourceVersion string, rf client.ResultFactory) {
	key := key(groupVersion, namespace, resource)
	if _, ok := w.watchers[key]; ok {
		return
	}
	ctx, cancel := context.WithCancel(w.ctx)
	w.watchers[key] = watchState{
		cancel: cancel,
	}
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		args := url.Values{}
		if resourceVersion != "" {
			args.Set("resourceVersion", resourceVersion)
		}
		for e := range w.client.Watch(ctx, groupVersion, namespace, resource, nil, args, rf) {
			select {
			case <-ctx.Done():
				return
			case w.events <- e:
			}
		}
	}()
}

func (w *Watcher) Forget(groupVersion, namespace, resource string) {
	key := key(groupVersion, namespace, resource)
	if current, ok := w.watchers[key]; ok {
		current.cancel()
		delete(w.watchers, key)
	}
}

func key(groupVersion, namespace, resource string) string {
	return fmt.Sprintf("%s|%s|%s", groupVersion, namespace, resource)
}
