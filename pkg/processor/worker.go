package processor

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/atlassian/smith"
	"github.com/atlassian/smith/pkg/resources"

	"github.com/cenk/backoff"
	"k8s.io/apimachinery/pkg/api/errors"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type worker struct {
	tp *TemplateProcessor
	bo backoff.BackOff
	workerRef

	// The fields below are protected by tp.lock

	// It is ok to mutate the template itself, but pointer should only be updated while locked.
	template *smith.Template
}

func (wrk *worker) rebuildLoop() {
	defer wrk.tp.wg.Done()
	defer wrk.cleanupState()

	for {
		tmpl := wrk.checkRebuild()
		if tmpl == nil {
			return
		}
		err := wrk.rebuild(tmpl)
		if err == nil {
			wrk.bo.Reset() // reset the backoff on successful rebuild
			// Make sure template does not need to be rebuilt before exiting goroutine by doing one more iteration
		} else if !wrk.handleError(tmpl, err) {
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
func (wrk *worker) rebuild(tmpl *smith.Template) error {
	if tmpl.Status.State == smith.TERMINAL_ERROR {
		// Sad, but true.
		return nil
	}
	log.Printf("[WORKER] Rebuilding the template %s/%s", wrk.namespace, wrk.tmplName)

	for _, res := range tmpl.Spec.Resources {
		if wrk.checkNeedsRebuild() {
			return nil
		}
		isReady, err := wrk.checkResource(tmpl, &res)
		if err != nil {
			return err
		}
		if !isReady {
			if err := wrk.setTemplateState(tmpl, smith.IN_PROGRESS); err != nil {
				log.Printf("[WORKER] %v", err)
			}
			return nil
		}
	}
	return wrk.setTemplateState(tmpl, smith.READY)
}

func (wrk *worker) checkResource(tmpl *smith.Template, res *smith.Resource) (isReady bool, e error) {
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
	res.Spec.SetLabels(mergeLabels(
		tmpl.Metadata.Labels,
		res.Spec.GetLabels(),
		map[string]string{smith.TemplateNameLabel: wrk.tmplName}))

	// 1. Update OwnerReferences
	// Hardcode APIVersion/Kind because of https://github.com/kubernetes/client-go/issues/60
	res.Spec.SetOwnerReferences(append(res.Spec.GetOwnerReferences(), metav1.OwnerReference{
		APIVersion: smith.TemplateResourceVersion,
		Kind:       smith.TemplateResourceKind,
		Name:       tmpl.Metadata.Name,
		UID:        tmpl.Metadata.UID,
	}))

	name := res.Spec.GetName()
	var response *unstructured.Unstructured
	for {
		// 2. Try to get the resource. We do read first to avoid generating unnecessary events.
		response, err = resClient.Get(name)
		if err != nil {
			if !errors.IsNotFound(err) {
				// Unexpected error
				return false, err
			}
			log.Printf("[WORKER] template %s/%s: resource %q not found, creating", wrk.namespace, wrk.tmplName, res.Name)
			// 3. Create if does not exist
			response, err = resClient.Create(&res.Spec)
			if err == nil {
				log.Printf("[WORKER] template %s/%s: resource %q created", wrk.namespace, wrk.tmplName, res.Name)
				break
			}
			if errors.IsAlreadyExists(err) {
				log.Printf("[WORKER] template %s/%s: resource %q found, restarting loop", wrk.namespace, wrk.tmplName, res.Name)
				continue
			}
			// Unexpected error
			return false, err
		}

		// 4. Compare spec and existing resource
		if isEqualResources(res, response) {
			//log.Printf("[WORKER] template %s/%s: resource %s has correct spec", wrk.namespace, wrk.tmplName, res.Name)
			break
		}

		// 5. Update if different
		// https://github.com/kubernetes/community/blob/master/contributors/devel/api-conventions.md#concurrency-control-and-consistency
		res.Spec.SetResourceVersion(response.GetResourceVersion()) // Do CAS

		response, err = resClient.Update(&res.Spec)
		if err != nil {
			if errors.IsConflict(err) {
				log.Printf("[WORKER] template %s/%s: resource %q update resulted in conflict, restarting loop", wrk.namespace, wrk.tmplName, res.Name)
				continue
			}
			// Unexpected error
			return false, err
		}
		log.Printf("[WORKER] template %s/%s: resource %q updated", wrk.namespace, wrk.tmplName, res.Name)
		break
	}
	return wrk.tp.rc.IsReady(response)
}

func (wrk *worker) setTemplateState(tpl *smith.Template, desired smith.ResourceState) error {
	if tpl.Status.State == desired {
		return nil
	}
	log.Printf("[WORKER] setting template %s/%s State to %q from %q", wrk.namespace, wrk.tmplName, desired, tpl.Status.State)
	tpl.Status.State = desired
	err := wrk.tp.templateClient.Put().
		Namespace(wrk.namespace).
		Resource(smith.TemplateResourcePath).
		Name(wrk.tmplName).
		Body(tpl).
		Do().
		Into(tpl)
	if err != nil {
		if kerrors.IsConflict(err) {
			// Something updated the template concurrently.
			// It is possible that it was us in previous iteration but we haven't observed the
			// resulting update event for the template and this iteration was triggered by something
			// else e.g. resource update.
			// It is safe to ignore this conflict because we will reiterate because of the update event.
			return nil
		}
		return fmt.Errorf("failed to set template %s/%s state to %q: %v", wrk.namespace, wrk.tmplName, desired, err)
	}
	log.Printf("[WORKER] template %s/%s is %q", wrk.namespace, wrk.tmplName, desired)
	// FIXME for some reason TypeMeta is not deserialized properly
	//log.Printf("[WORKER] Into = %#v", tpl)
	return nil
}

// checkRebuild returns a pointer to the template if a rebuild is required.
// It returns nil if there is no new template and rebuild is not required.
func (wrk *worker) checkRebuild() *smith.Template {
	wrk.tp.lock.Lock()
	defer wrk.tp.lock.Unlock()
	tmpl := wrk.template
	if tmpl != nil {
		wrk.template = nil
		return tmpl
	}
	// delete atomically with the check to avoid race with Processor's rebuildInternal()
	delete(wrk.tp.workers, wrk.workerRef)
	return nil
}

func (wrk *worker) cleanupState() {
	wrk.tp.lock.Lock()
	defer wrk.tp.lock.Unlock()
	if wrk.tp.workers[wrk.workerRef] == wrk {
		// Only cleanup if there is a stale reference to the current worker.
		delete(wrk.tp.workers, wrk.workerRef)
	}
}

func (wrk *worker) handleError(tmpl *smith.Template, err error) (shouldContinue bool) {
	if err == context.Canceled || err == context.DeadlineExceeded {
		return false
	}
	log.Printf("[WORKER] Failed to rebuild the template %s/%s: %v", wrk.namespace, wrk.tmplName, err)
	next := wrk.bo.NextBackOff()
	if next == backoff.Stop {
		if e := wrk.setTemplateState(tmpl, smith.TERMINAL_ERROR); e != nil {
			log.Printf("[WORKER] %v", e)
		}
		return false
	}
	func() {
		wrk.tp.lock.Lock()
		defer wrk.tp.lock.Unlock()
		wrk.template = tmpl
	}()
	if e := wrk.setTemplateState(tmpl, smith.ERROR); e != nil {
		log.Printf("[WORKER] %v", e)
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
	return wrk.template != nil
}

// isEqualResources checks that existing resource matches the desired spec.
func isEqualResources(res *smith.Resource, spec *unstructured.Unstructured) bool {
	// TODO implement
	// ignore additional annotations/labels? or make the merge behaviour configurable?
	return true
}

func mergeLabels(labels ...map[string]string) map[string]string {
	result := make(map[string]string)
	for _, m := range labels {
		for k, v := range m {
			result[k] = v
		}
	}
	return result
}
