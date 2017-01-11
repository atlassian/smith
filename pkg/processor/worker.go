package processor

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/atlassian/smith"
	"github.com/atlassian/smith/pkg/resources"

	"github.com/cenk/backoff"
	"k8s.io/client-go/pkg/api/errors"
	metav1 "k8s.io/client-go/pkg/apis/meta/v1"
	"k8s.io/client-go/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/pkg/runtime/schema"
)

type worker struct {
	tp *TemplateProcessor
	bo backoff.BackOff
	workerRef

	// The fields below are protected by tp.lock

	// It is ok to mutate the template itself, but pointer should only be updated while locked.
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
	gv, err := schema.ParseGroupVersion(res.Spec.GetAPIVersion())
	if err != nil {
		return false, err
	}
	kind := res.Spec.GetKind()
	client, err := wrk.tp.clients.ClientForGroupVersionKind(gv.WithKind(kind))
	if err != nil {
		return false, err
	}

	resClient := client.Resource(&metav1.APIResource{
		Name:       resources.ResourceKindToPath(kind),
		Namespaced: true,
		Kind:       kind,
	}, wrk.namespace)

	// 0. Update label to point at the parent template
	labels := res.Spec.GetLabels()
	if labels == nil {
		labels = make(map[string]string)
		res.Spec.SetLabels(labels)
	}
	labels[smith.TemplateNameLabel] = wrk.name
	name := res.Spec.GetName()
	for {
		var response *unstructured.Unstructured
		// 1. Try to get the resource. We do read first to avoid generating unnecessary events.
		//err := wrk.tp.client.Get(wrk.tp.ctx, res.Spec.APIVersion, wrk.namespace, resourcePath, res.Spec.Name, nil, &response)
		response, err := resClient.Get(name)
		if err != nil {
			if !errors.IsNotFound(err) {
				// Unexpected error
				return false, err
			}
			log.Printf("template %s/%s: resource %s not found, creating", wrk.namespace, wrk.name, res.Name)
			// 2. Create if does not exist
			response, err = resClient.Create(&res.Spec)
			if err == nil {
				log.Printf("template %s/%s: resource %s created", wrk.namespace, wrk.name, res.Name)
				break
			}
			if errors.IsAlreadyExists(err) {
				log.Printf("template %s/%s: resource %s found, restarting loop", wrk.namespace, wrk.name, res.Name)
				continue
			}
			// Unexpected error
			return false, err
		}

		// 3. Compare spec and existing resource
		if isEqualResources(res, response) {
			log.Printf("template %s/%s: resource %s has correct spec", wrk.namespace, wrk.name, res.Name)
			break
		}

		// 4. Update if different
		// https://github.com/kubernetes/community/blob/master/contributors/devel/api-conventions.md#concurrency-control-and-consistency
		res.Spec.SetResourceVersion(response.GetResourceVersion()) // Do CAS

		response, err = resClient.Update(&res.Spec)
		if err != nil {
			if errors.IsConflict(err) {
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

// TODO use cache.Store from reflector instead of this method
func (wrk *worker) fetchTemplate() (*smith.Template, error) {
	log.Printf("Fetching the template %s/%s", wrk.namespace, wrk.name)
	tpl := new(smith.Template)
	err := wrk.tp.templateClient.Get().
		Namespace(wrk.namespace).
		Resource(smith.TemplateResourcePath).
		Name(wrk.name).
		Do().
		Into(tpl)
	if err != nil {
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
		// TODO figure out which one is more fresh by comparing resource versions?
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
	err := wrk.tp.templateClient.Put().
		Namespace(wrk.namespace).
		Resource(smith.TemplateResourcePath).
		Name(wrk.name).
		Body(tpl).
		Do().
		Into(tpl)
	if err != nil {
		return fmt.Errorf("failed to set template %s/%s state to %s: %v", wrk.namespace, wrk.name, desired, err)
	}
	// FIXME for some reason TypeMeta is not deserialized properly
	log.Printf("Into = %#v", tpl)
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
func isEqualResources(res *smith.Resource, spec *unstructured.Unstructured) bool {
	// TODO implement
	// ignore additional annotations/labels? or make the merge behaviour configurable?
	return true
}
