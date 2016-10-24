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

type ReadyChecker interface {
	IsReady(*smith.Resource) (bool, error)
}

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
	rc      ReadyChecker
	wg      sync.WaitGroup // tracks number of Goroutines running rebuildLoop()

	lock      sync.RWMutex // protects fields below
	templates map[templateRef]*templateState
}

// New creates a new template processor.
// Instances are safe for concurrent use.
func New(ctx context.Context, client *client.ResourceClient, rc ReadyChecker) *TemplateProcessor {
	return &TemplateProcessor{
		ctx:       ctx,
		backoff:   exponentialBackOff,
		client:    client,
		rc:        rc,
		templates: make(map[templateRef]*templateState),
	}
}

func (tp *TemplateProcessor) Join() {
	tp.wg.Wait()
}

// Rebuild schedules a rebuild of the template.
// Note that the template object and/or resources in the template may be mutated asynchronously so the
// calling code should do a proper deep copy if the object is still needed.
func (tp *TemplateProcessor) Rebuild(tpl *smith.Template) {
	tp.rebuildInternal(tpl.Namespace, tpl.Name, tpl)
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
	var err error
	if tpl.Status.State != smith.READY {
		tpl.Status.State = smith.READY
		err = tp.client.Update(tp.ctx, smith.TemplateResourceGroupVersion, namespace, smith.TemplateResourcePath, name, tpl, nil)
	}
	if err == nil {
		log.Printf("Template %s/%s is READY", namespace, name)
	}
	return err
}

func (tp *TemplateProcessor) checkResource(namespace, templateName string, res *smith.Resource) (isReady bool, e error) {
	resourcePath := client.ResourceKindToPath(res.Spec.Kind)
	// 0. Update label to point at the parent template
	if res.Spec.Labels == nil {
		res.Spec.Labels = make(map[string]string)
	}
	res.Spec.Labels[smith.TemplateNameLabel] = templateName
	for {
		var response smith.ResourceSpec

		// 1. Try to get the resource. We do read first to avoid generating unnecessary events.
		err := tp.client.Get(tp.ctx, res.Spec.APIVersion, namespace, resourcePath, res.Spec.Name, nil, &response)
		if err != nil {
			if !client.IsNotFound(err) {
				// Unexpected error
				return false, err
			}
			log.Printf("template %s/%s: resource %s not found, creating", namespace, templateName, res.Name)
			// 2. Create if does not exist
			err = tp.client.Create(tp.ctx, res.Spec.APIVersion, namespace, resourcePath, &res.Spec, &response)
			if err == nil {
				log.Printf("template %s/%s: resource %s created", namespace, templateName, res.Name)
				break
			}
			if client.IsAlreadyExists(err) {
				log.Printf("template %s/%s: resource %s found, restarting loop", namespace, templateName, res.Name)
				continue
			}
			// Unexpected error
			return false, err
		}

		// 3. Compare spec and existing resource
		if isEqualResources(res, &response) {
			log.Printf("template %s/%s: resource %s has correct spec", namespace, templateName, res.Name)
			break
		}

		// 4. Update if different
		// https://github.com/kubernetes/kubernetes/blob/master/docs/devel/api-conventions.md#concurrency-control-and-consistency
		res.Spec.ResourceVersion = response.ResourceVersion // Do CAS
		err = tp.client.Update(tp.ctx, res.Spec.APIVersion, namespace, resourcePath, res.Spec.Name, &res.Spec, &response)
		if err != nil {
			if client.IsConflict(err) {
				log.Printf("template %s/%s: resource %s update resulted in conflict, restarting loop", namespace, templateName, res.Name)
				continue
			}
			// Unexpected error
			return false, err
		}
		log.Printf("template %s/%s: resource %s updated", namespace, templateName, res.Name)
		break
	}
	return tp.rc.IsReady(res)
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

// isEqualResources checks that existing resource matches the desired spec.
func isEqualResources(res *smith.Resource, spec *smith.ResourceSpec) bool {
	// TODO implement
	// ignore additional annotations/labels? or make the merge behaviour configurable?
	return true
}

func exponentialBackOff() backoff.BackOff {
	b := backoff.NewExponentialBackOff()
	b.InitialInterval = 2 * time.Second
	b.MaxElapsedTime = time.Duration(math.MaxInt64)
	return b
}
