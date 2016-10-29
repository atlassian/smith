package processor

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/atlassian/smith"
	"github.com/atlassian/smith/pkg/client"

	"github.com/cenk/backoff"
)

type worker struct {
	tp *TemplateProcessor
	bo backoff.BackOff
	workerRef

	// These fields are protected with tp.lock
	template     *smith.Template
	needsRebuild bool
}

func (wrk *worker) rebuildLoop() {
	defer wrk.tp.wg.Done()
	defer wrk.cleanupState()

	for {
		tpl, needsRebuild := wrk.checkRebuild()
		if !needsRebuild {
			return
		}
		err := wrk.rebuild(tpl)
		if err == nil {
			wrk.bo.Reset() // reset the backoff on successful rebuild
			// Make sure template does not need to be rebuilt before exiting goroutine by doing one more iteration
		} else if !wrk.handleError(tpl, err) {
			return
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
func (wrk *worker) rebuild(tpl *smith.Template) error {
	if tpl == nil {
		var err error
		tpl, err = wrk.fetchTemplate()
		if err != nil {
			return err
		}
	}
	if tpl.Status.State == smith.TERMINAL_ERROR {
		// Sad, but true.
		return nil
	}
	log.Printf("Rebuilding the template %s/%s", wrk.namespace, wrk.name)

	for _, res := range tpl.Spec.Resources {
		if wrk.checkNeedsRebuild() {
			return nil
		}
		isReady, err := wrk.checkResource(&res)
		if err != nil {
			return err
		}
		if !isReady {
			if err := wrk.setTemplateState(tpl, smith.IN_PROGRESS); err != nil {
				log.Printf("%v", err)
			}
			return nil
		}
	}
	err := wrk.setTemplateState(tpl, smith.READY)
	if err == nil {
		log.Printf("Template %s/%s is %s", wrk.namespace, wrk.name, smith.READY)
	}
	return err
}

func (wrk *worker) checkResource(res *smith.Resource) (isReady bool, e error) {
	resourcePath := client.ResourceKindToPath(res.Spec.Kind)
	// 0. Update label to point at the parent template
	if res.Spec.Labels == nil {
		res.Spec.Labels = make(map[string]string)
	}
	res.Spec.Labels[smith.TemplateNameLabel] = wrk.name
	for {
		var response smith.ResourceSpec

		// 1. Try to get the resource. We do read first to avoid generating unnecessary events.
		err := wrk.tp.client.Get(wrk.tp.ctx, res.Spec.APIVersion, wrk.namespace, resourcePath, res.Spec.Name, nil, &response)
		if err != nil {
			if !client.IsNotFound(err) {
				// Unexpected error
				return false, err
			}
			log.Printf("template %s/%s: resource %s not found, creating", wrk.namespace, wrk.name, res.Name)
			// 2. Create if does not exist
			err = wrk.tp.client.Create(wrk.tp.ctx, res.Spec.APIVersion, wrk.namespace, resourcePath, &res.Spec, &response)
			if err == nil {
				log.Printf("template %s/%s: resource %s created", wrk.namespace, wrk.name, res.Name)
				break
			}
			if client.IsAlreadyExists(err) {
				log.Printf("template %s/%s: resource %s found, restarting loop", wrk.namespace, wrk.name, res.Name)
				continue
			}
			// Unexpected error
			return false, err
		}

		// 3. Compare spec and existing resource
		if isEqualResources(res, &response) {
			log.Printf("template %s/%s: resource %s has correct spec", wrk.namespace, wrk.name, res.Name)
			break
		}

		// 4. Update if different
		// https://github.com/kubernetes/kubernetes/blob/master/docs/devel/api-conventions.md#concurrency-control-and-consistency
		res.Spec.ResourceVersion = response.ResourceVersion // Do CAS
		err = wrk.tp.client.Update(wrk.tp.ctx, res.Spec.APIVersion, wrk.namespace, resourcePath, res.Spec.Name, &res.Spec, &response)
		if err != nil {
			if client.IsConflict(err) {
				log.Printf("template %s/%s: resource %s update resulted in conflict, restarting loop", wrk.namespace, wrk.name, res.Name)
				continue
			}
			// Unexpected error
			return false, err
		}
		log.Printf("template %s/%s: resource %s updated", wrk.namespace, wrk.name, res.Name)
		break
	}
	return wrk.tp.rc.IsReady(res)
}

func (wrk *worker) fetchTemplate() (*smith.Template, error) {
	log.Printf("Fetching the template %s/%s", wrk.namespace, wrk.name)
	tpl := new(smith.Template)
	if err := wrk.tp.client.Get(wrk.tp.ctx, smith.TemplateResourceGroupVersion, wrk.namespace, smith.TemplateResourcePath, wrk.name, nil, tpl); err != nil {
		// TODO handle 404 - template was deleted
		return nil, err
	}
	// Store fetched template for future reference
	wrk.tp.lock.Lock()
	defer wrk.tp.lock.Unlock()
	if wrk.template == nil {
		wrk.template = tpl
	} else {
		// Do not overwrite template that came via event while we were fetching it - it may be more fresh.
		tpl = wrk.template
	}
	wrk.needsRebuild = false // In any case we are using the latest template, no need for rebuilds
	return tpl, nil
}

func (wrk *worker) setTemplateState(tpl *smith.Template, desired smith.ResourceState) error {
	if tpl.Status.State == desired {
		return nil
	}
	tpl.Status.State = desired
	err := wrk.tp.client.Update(wrk.tp.ctx, smith.TemplateResourceGroupVersion, wrk.namespace, smith.TemplateResourcePath, wrk.name, tpl, nil)
	if err != nil {
		return fmt.Errorf("failed to set template %s/%s state to %s: %v", wrk.namespace, wrk.name, desired, err)
	}
	return nil
}

// checkRebuild gets the current "rebuild required" status and resets it.
// Also a pointer to the template is returned which may be nil.
func (wrk *worker) checkRebuild() (*smith.Template, bool) {
	wrk.tp.lock.Lock()
	defer wrk.tp.lock.Unlock()
	if wrk.needsRebuild {
		wrk.needsRebuild = false
		return wrk.template, true
	}
	// delete atomically with the check to avoid race with Processor's rebuildInternal()
	delete(wrk.tp.workers, wrk.workerRef)
	return nil, false
}

func (wrk *worker) cleanupState() {
	wrk.tp.lock.Lock()
	defer wrk.tp.lock.Unlock()
	delete(wrk.tp.workers, wrk.workerRef)
}

func (wrk *worker) handleError(tpl *smith.Template, err error) (shouldContinue bool) {
	if err == context.Canceled || err == context.DeadlineExceeded {
		return false
	}
	log.Printf("Failed to rebuild the template %s/%s: %v", wrk.namespace, wrk.name, err)
	next := wrk.bo.NextBackOff()
	if next == backoff.Stop {
		if e := wrk.setTemplateState(tpl, smith.TERMINAL_ERROR); e != nil {
			log.Printf("%v", e)
		}
		return false
	}
	func() {
		wrk.tp.lock.Lock()
		defer wrk.tp.lock.Unlock()
		wrk.needsRebuild = true
	}()
	if e := wrk.setTemplateState(tpl, smith.ERROR); e != nil {
		log.Printf("%v", e)
	}
	after := time.NewTimer(next)
	defer after.Stop()
	select {
	case <-wrk.tp.ctx.Done():
		return false
	case <-after.C:
	}
	return true
}

// needsRebuild can be called inside of the rebuild loop to check if the template needs to be rebuilt from the start.
func (wrk *worker) checkNeedsRebuild() bool {
	wrk.tp.lock.RLock()
	defer wrk.tp.lock.RUnlock()
	return wrk.needsRebuild
}

// isEqualResources checks that existing resource matches the desired spec.
func isEqualResources(res *smith.Resource, spec *smith.ResourceSpec) bool {
	// TODO implement
	// ignore additional annotations/labels? or make the merge behaviour configurable?
	return true
}
