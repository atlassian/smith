package readychecker

import (
	"fmt"
	"log"

	"github.com/atlassian/smith"
	"github.com/atlassian/smith/pkg/resources"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ext_v1b1 "k8s.io/client-go/pkg/apis/extensions/v1beta1"
)

// IsObjectReady checks if an object is Ready.
// Each function is responsible for handling different versions of objects itself.
type IsObjectReady func(*unstructured.Unstructured) (isReady, retriableError bool, e error)

// TprStore gets a TPR definition for a Group and Kind of the resource (TPR instance).
// Returns nil if TPR definition was not found.
type TprStore interface {
	Get(resource schema.GroupKind) (*ext_v1b1.ThirdPartyResource, error)
}

type ReadyChecker struct {
	Store      TprStore
	KnownTypes map[schema.GroupKind]IsObjectReady
}

func New(store TprStore, kts ...map[schema.GroupKind]IsObjectReady) *ReadyChecker {
	kt := make(map[schema.GroupKind]IsObjectReady)
	for _, knownTypes := range kts {
		for knownGK, f := range knownTypes {
			if kt[knownGK] != nil {
				panic(fmt.Errorf("GK specified more than once: %s", knownGK))
			}
			kt[knownGK] = f
		}
	}
	return &ReadyChecker{
		Store:      store,
		KnownTypes: kt,
	}
}

func (rc *ReadyChecker) IsReady(obj *unstructured.Unstructured) (isReady, retriableError bool, e error) {
	gvk := obj.GroupVersionKind()
	gk := gvk.GroupKind()

	if gk.Kind == "" || gvk.Version == "" { // Group can be empty e.g. built-in objects like ConfigMap
		return false, false, fmt.Errorf("object %q has empty kind/version: %v", obj.GetName(), gvk)
	}

	// 1. Check if it is a known built-in resource
	if isObjectReady, ok := rc.KnownTypes[gk]; ok {
		return isObjectReady(obj)
	}

	// 2. Check if it is a TPR with path/value annotation
	ready, retriable, err := rc.checkPathValue(gk, obj)
	if err != nil || ready {
		return ready, retriable, err
	}

	// 3. Check if it is a TPR with Kind/GroupVersion annotation
	return rc.checkForInstance(gk, obj)
}

func (rc *ReadyChecker) checkForInstance(gk schema.GroupKind, obj *unstructured.Unstructured) (isReady, retriableError bool, e error) {
	// TODO Check if it is a TPR with Kind/GroupVersion annotation
	return false, false, nil
}

func (rc *ReadyChecker) checkPathValue(gk schema.GroupKind, obj *unstructured.Unstructured) (isReady, retriableError bool, e error) {
	tpr, err := rc.Store.Get(gk)
	if err != nil {
		return false, true, err
	}
	if tpr == nil {
		return false, false, nil
	}
	path := tpr.Annotations[smith.TprFieldPathAnnotation]
	value := tpr.Annotations[smith.TprFieldValueAnnotation]
	if len(path) == 0 || len(value) == 0 {
		return false, false, nil
	}
	actualValue, err := resources.GetJsonPathString(obj.Object, path)
	if err != nil {
		return false, false, err
	}
	if actualValue != value {
		// TODO this is for debugging, remove later
		log.Printf("[IsReady] %q is not equal to expected %q", actualValue, value)
		return false, false, nil
	}
	log.Printf("[IsReady] %q is equal to expected %q", actualValue, value)
	return true, false, nil
}
