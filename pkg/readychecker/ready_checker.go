package readychecker

import (
	"errors"
	"log"
	"strings"

	"github.com/atlassian/smith"

	extensions "k8s.io/client-go/pkg/apis/extensions/v1beta1"
	"k8s.io/client-go/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/pkg/runtime/schema"
)

var UnknownResourceGroupKind = errors.New("unknown resource group and/or kind")

var alwaysReady = map[schema.GroupKind]struct{}{
	schema.GroupKind{"", "ConfigMap"}: {},
	schema.GroupKind{"", "Secret"}:    {},
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

	// 3. TODO Check if it is a TPR with Kind/GroupVersion annotation

	// 4. Nothing has been found
	if err == nil {
		err = UnknownResourceGroupKind
	}

	return false, err
}

func (rc *ReadyChecker) checkPathValue(gk schema.GroupKind, obj *unstructured.Unstructured) (bool, error) {
	tpr, err := rc.store.Get(gk)
	if err == nil && tpr != nil {
		path := tpr.Annotations[smith.TprFieldPathAnnotation]
		value := tpr.Annotations[smith.TprFieldValueAnnotation]
		if len(path) > 0 && len(value) > 0 {
			actualValue := getNestedString(obj.Object, strings.Split(path, ".")...)
			if actualValue == value {
				return true, nil
			} else {
				// TODO this is for debugging, remove later
				log.Printf("IsReady: %q is not equal expected %q", actualValue, value)
			}
		}
	}
	return false, err
}
