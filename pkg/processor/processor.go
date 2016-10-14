package processor

import (
	"context"
	"log"
	"math"
	"sync"
	"time"

	"github.com/atlassian/smith"
	"github.com/atlassian/smith/pkg/client"

	"github.com/cenk/backoff"
)

type BackOffFactory func() backoff.BackOff

type templateState struct {
	template     *smith.Template
	needsRebuild bool
}

type templateRef struct {
	namespace string
	name      string
}

type TemplateProcessor struct {
	ctx     context.Context
	backoff BackOffFactory
	client  *client.ResourceClient
	wg      sync.WaitGroup // tracks number of Goroutines running rebuildLoop()

	lock      sync.RWMutex // protects fields below
	templates map[templateRef]*templateState
}

// New creates a new template processor.
// Instances are safe for concurrent use.
func New(ctx context.Context, client *client.ResourceClient) *TemplateProcessor {
	return &TemplateProcessor{
		ctx:       ctx,
		backoff:   exponentialBackOff,
		client:    client,
		templates: make(map[templateRef]*templateState),
	}
}

func (tp *TemplateProcessor) Join() {
	tp.wg.Wait()
}

func (tp *TemplateProcessor) Rebuild(tpl smith.Template) { // make a copy
	tp.rebuildInternal(tpl.Namespace, tpl.Name, &tpl)
}

func (tp *TemplateProcessor) RebuildByName(namespace, name string) {
	tp.rebuildInternal(namespace, name, nil)
}

func (tp *TemplateProcessor) rebuildInternal(namespace, name string, tpl *smith.Template) {
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
		go tp.rebuildLoop(namespace, name)
	} else {
		state.template = tpl
		state.needsRebuild = true
	}
}

func (tp *TemplateProcessor) rebuildLoop(namespace, name string) {
	defer tp.wg.Done()
	bo := tp.backoff()
	for {
		tpl, needsRebuild := tp.checkRebuild(namespace, name)
		if !needsRebuild {
			// TODO cleanup state in tp on OTHER exit conditions?
			return
		}
		err := tp.rebuild(namespace, name, tpl)
		if err == nil {
			bo.Reset() // reset the backoff on successful rebuild
			continue   // Make sure template does not need to be rebuilt before exiting goroutine by doing one more loop
		}
		if err == context.Canceled || err == context.DeadlineExceeded {
			return
		}
		log.Printf("Failed to rebuild the template %s/%s: %v", namespace, name, err)
		next := bo.NextBackOff()
		if next == backoff.Stop {
			// TODO should have limited number of retries?
			return
		}
		// TODO restore needsRebuild flag or record the last error/etc to make checkRebuild() return true
		// Update template Status with the error message?
		after := time.NewTimer(next)
		select {
		case <-tp.ctx.Done():
			after.Stop()
			return
		case <-after.C:
		}
	}
}

// TODO parse template, build resource graph, traverse graph, assert each resource exists.
// For each resource ensure its dependencies (if any) are in READY state before creating it.
// If at least one dependency is not READY, exit loop. Rebuild will/should be called once the dependency
// updates it's state (noticed via watching).

// READY state might mean something different for each resource type. For ThirdPartyResources it may mean
// that a field "State" in the Status of the resource is set to "Ready". It may be customizable via
// annotations with some defaults.
func (tp *TemplateProcessor) rebuild(namespace, name string, tpl *smith.Template) error {
	if tpl == nil {
		var err error
		tpl, err = tp.fetchTemplate(namespace, name)
		if err != nil {
			return err
		}
	}
	log.Printf("Rebuilding the template %s/%s", namespace, name)

	for _, res := range tpl.Spec.Resources {
		if tp.needsRebuild(namespace, name) {
			return nil
		}
		isReady, err := tp.checkResource(namespace, name, &res)
		if err != nil || !isReady {
			return err
		}
	}

	return nil
	//tp.client.Update(tp.ctx, smith.TemplateResourceGroupVersion, namespace, smith.TemplateResourcePath, name, &smith.Template{
	//	TypeMeta: smith.TypeMeta{
	//		Kind:       smith.TemplateResourceKind,
	//		APIVersion: smith.TemplateResourceGroupVersion,
	//	},
	//	ObjectMeta: smith.ObjectMeta{
	//		Name: name,
	//		ResourceVersion: tpl.ResourceVersion,
	//	},
	//	Status: smith.TemplateStatus{
	//		ResourceStatus: smith.ResourceStatus{
	//			State: smith.READY,
	//		},
	//	},
	//}, nil)
}

func (tp *TemplateProcessor) checkResource(namespace, templateName string, res *smith.Resource) (isReady bool, e error) {
	for {
		var response smith.ResourceSpec
		resourcePath := client.ResourceKindToPath(res.Spec.Kind)
		// 0. TODO Update label to point at the parent template

		// 1. Try to create resource
		err := tp.client.Create(tp.ctx, res.Spec.APIVersion, namespace, resourcePath, &res.Spec, &response)
		if err == nil {
			log.Printf("template %s/%s: resource %s created", namespace, templateName, res.Name)
			return false, nil
		}
		if !client.IsAlreadyExists(err) {
			// Unexpected error
			return false, err
		}
		// Resource already exists
		// 2. Get the existing resource
		err = tp.client.Get(tp.ctx, res.Spec.APIVersion, namespace, resourcePath, res.Spec.Name, nil, &response)
		if err != nil {
			if client.IsNotFound(err) {
				log.Printf("template %s/%s: resource %s not found, restarting loop", namespace, templateName, res.Name)
				continue
			}
			// Unexpected error
			return false, err
		}
		// 3. TODO Compare (ignore additional annotations/labels, status?)

		// 4. TODO Update if different

		return true, nil
	}
}

func (tp *TemplateProcessor) fetchTemplate(namespace, name string) (*smith.Template, error) {
	log.Printf("Fetching the template %s/%s", namespace, name)
	var tpl smith.Template
	if err := tp.client.Get(tp.ctx, smith.TemplateResourceGroupVersion, namespace, smith.TemplateResourcePath, name, nil, &tpl); err != nil {
		// TODO handle 404 - template was deleted
		return nil, err
	}
	// Store fetched template for future reference
	ref := templateRef{namespace: namespace, name: name}
	tp.lock.Lock()
	defer tp.lock.Unlock()
	tp.templates[ref].template = &tpl
	return &tpl, nil
}

// checkRebuild gets the current "rebuild required" status and resets it.
// Also a pointer to the template is returned which may be nil.
func (tp *TemplateProcessor) checkRebuild(namespace, name string) (*smith.Template, bool) {
	ref := templateRef{namespace: namespace, name: name}
	tp.lock.Lock()
	defer tp.lock.Unlock()
	state := tp.templates[ref]
	if state.needsRebuild {
		state.needsRebuild = false
		return state.template, true
	}
	delete(tp.templates, ref)
	return nil, false
}

// needsRebuild can be called inside of the rebuild loop to check if the template needs to be rebuilt from the start.
func (tp *TemplateProcessor) needsRebuild(namespace, name string) bool {
	ref := templateRef{namespace: namespace, name: name}
	tp.lock.RLock()
	defer tp.lock.RUnlock()
	ts := tp.templates[ref]
	if ts == nil {
		return false
	}
	return ts.needsRebuild
}

func exponentialBackOff() backoff.BackOff {
	b := backoff.NewExponentialBackOff()
	b.MaxElapsedTime = time.Duration(math.MaxInt64)
	return b
}
