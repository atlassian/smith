package processor

import (
	"sync"

	"github.com/ash2k/smith"
	"github.com/ash2k/smith/pkg/client"
)

type templateState struct {
	template     smith.Template
	needsRebuild bool
}

type templateProcessor struct {
	client *client.ResourceClient
	// protects fields below
	lock      sync.Mutex
	templates map[string]*templateState
}

// New creates a new template processor.
// Instances are safe for concurrent use.
func New(client *client.ResourceClient) *templateProcessor {
	return &templateProcessor{
		client:    client,
		templates: make(map[string]*templateState),
	}
}

func (tp *templateProcessor) Rebuild(tpl *smith.Template) {
	tp.lock.Lock()
	defer tp.lock.Unlock()
	state := tp.templates[tpl.Name]
	if state == nil {
		state = &templateState{
			template:     *tpl,
			needsRebuild: true,
		}
		tp.templates[tpl.Name] = state
		go tp.rebuild(tpl.Name)
	} else {
		state.template = *tpl
		state.needsRebuild = true
	}
}

func (tp *templateProcessor) rebuild(name string) {
	for {
		var tpl smith.Template
		needsRebuild := func() bool {
			tp.lock.Lock()
			defer tp.lock.Unlock()
			state := tp.templates[name]
			if state.needsRebuild {
				tpl = state.template
				state.needsRebuild = false
				return true
			}
			delete(tp.templates, name)
			return false
		}()
		if !needsRebuild {
			break
		}
		// TODO parse template, build resource graph, traverse graph, assert each resource exists.
		// For each resource await its dependencies to reach READY state before creating it.

		// ....

		// Make sure resource does not need to be rebuilt before exiting goroutine by doing one more loop
	}
}

func (tp *templateProcessor) needsRebuild(name string) bool {
	tp.lock.Lock()
	defer tp.lock.Unlock()
	return tp.templates[name].needsRebuild
}
