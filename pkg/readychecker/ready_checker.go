package readychecker

import (
	"fmt"
	"log"
	"strings"

	"github.com/atlassian/smith"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	extensions "k8s.io/client-go/pkg/apis/extensions/v1beta1"
)

var alwaysReady = map[schema.GroupKind]struct{}{
	{Group: "", Kind: "ConfigMap"}: {},
	{Group: "", Kind: "Secret"}:    {},
}

// TprStore gets a TPR definition for a Group and Kind of the resource (TPR instance).
// Returns nil if TPR definition was not found.
type TprStore interface {
	Get(resource schema.GroupKind) (*extensions.ThirdPartyResource, error)
}

type ReadyChecker struct {
	store TprStore
}

func New(store TprStore) *ReadyChecker {
	return &ReadyChecker{
		store: store,
	}
}

func (rc *ReadyChecker) IsReady(obj *unstructured.Unstructured) (bool, error) {
	gk := obj.GroupVersionKind().GroupKind()

	// 1. Check if it is an always-ready resource
	if _, ok := alwaysReady[gk]; ok {
		return true, nil
	}

	// 2. Check if it is a TPR with path/value annotation
	ready, err := rc.checkPathValue(gk, obj)
	if err == nil && ready {
		return true, nil
	}

	// 3. Check if it is a TPR with Kind/GroupVersion annotation
	ready, err2 := rc.checkForInstance(gk, obj)
	if err2 == nil && ready {
		return true, nil
	}

	// 4. Nothing has been found
	if err == nil {
		err = err2
	}

	return false, err
}

func (rc *ReadyChecker) checkForInstance(gk schema.GroupKind, obj *unstructured.Unstructured) (bool, error) {
	// TODO Check if it is a TPR with Kind/GroupVersion annotation
	return false, nil
}

func (rc *ReadyChecker) checkPathValue(gk schema.GroupKind, obj *unstructured.Unstructured) (bool, error) {
	tpr, err := rc.store.Get(gk)
	if err != nil {
		return false, err
	}
	if tpr == nil {
		return false, fmt.Errorf("unknown resource group %q and/or kind %q", gk.Group, gk.Kind)
	}
	path := tpr.Annotations[smith.TprFieldPathAnnotation]
	value := tpr.Annotations[smith.TprFieldValueAnnotation]
	if len(path) == 0 || len(value) == 0 {
		return false, fmt.Errorf("TPR %q is not annotated propery", tpr.Name)
	}
	actualValue := getNestedString(obj.Object, strings.Split(path, ".")...)
	if actualValue != value {
		// TODO this is for debugging, remove later
		log.Printf("[IsReady] %q is not equal to expected %q", actualValue, value)
		return false, nil
	}
	log.Printf("[IsReady] %q is equal to expected %q", actualValue, value)
	return true, nil
}
