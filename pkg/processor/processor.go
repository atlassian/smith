package processor

import (
	"context"
	"sync"

	"github.com/ash2k/smith"
	"github.com/ash2k/smith/pkg/client"
)

type templateState struct {
	template     *smith.Template
	needsRebuild bool
}

type templateRef struct {
	namespace string
	name      string
}

type templateProcessor struct {
	ctx    context.Context
	client *client.ResourceClient
	wg     sync.WaitGroup // tracks number of running rebuild Goroutines
	// protects fields below
	lock      sync.RWMutex
	templates map[templateRef]*templateState
}

// New creates a new template processor.
// Instances are safe for concurrent use.
func New(ctx context.Context, client *client.ResourceClient) *templateProcessor {
	return &templateProcessor{
		ctx:       ctx,
		client:    client,
		templates: make(map[templateRef]*templateState),
	}
}

func (tp *templateProcessor) Join() {
	tp.wg.Wait()
}

func (tp *templateProcessor) Rebuild(tpl smith.Template) { // make a copy
	tp.rebuildInternal(tpl.Namespace, tpl.Name, &tpl)
}

func (tp *templateProcessor) RebuildByName(namespace, name string) {
	tp.rebuildInternal(namespace, name, nil)
}

func (tp *templateProcessor) rebuildInternal(namespace, name string, tpl *smith.Template) {
	ref := templateRef{namespace: namespace, name: name}
	tp.lock.Lock()
	defer tp.lock.Unlock()
	state := tp.templates[ref]
	if state == nil {
		state = &templateState{
			template:     tpl,
			needsRebuild: true,
		}
		tp.templates[ref] = state
		tp.wg.Add(1)
		go tp.rebuild(namespace, name)
	} else {
		state.template = tpl
		state.needsRebuild = true
	}
}

func (tp *templateProcessor) rebuild(namespace, name string) {
	defer tp.wg.Done()
	ref := templateRef{namespace: namespace, name: name}
	for {
		var tpl *smith.Template
		needsRebuild := func() bool {
			tp.lock.Lock()
			defer tp.lock.Unlock()
			state := tp.templates[ref]
			if state.needsRebuild {
				tpl = state.template
				state.needsRebuild = false
				return true
			}
			delete(tp.templates, ref)
			return false
		}()
		if !needsRebuild {
			break
		}
		if tpl == nil {
			// TODO fetch template
			func() {
				// Store fetched template for future reference
				tp.lock.Lock()
				defer tp.lock.Unlock()
				tp.templates[ref].template = tpl
			}()
		}
		// TODO parse template, build resource graph, traverse graph, assert each resource exists.
		// For each resource ensure its dependencies (if any) are in READY state before creating it.
		// If at least one dependency is not READY, exit loop. Rebuild will/should be called once the dependency
		// updates it's state (noticed via watching).

		// READY state might mean something different for each resource type. For ThirdPartyResources it may mean
		// that a field "State" in the Status of the resource is set to "Ready". It may be customizable via
		// annotations with some defaults.

		// ....

		// Make sure template does not need to be rebuilt before exiting goroutine by doing one more loop
	}
}

// needsRebuild can be called inside of the rebuild loop to check if the template needs to be rebuilt from the start.
func (tp *templateProcessor) needsRebuild(namespace, name string) bool {
	ref := templateRef{namespace: namespace, name: name}
	tp.lock.RLock()
	defer tp.lock.RUnlock()
	return tp.templates[ref].needsRebuild
}
