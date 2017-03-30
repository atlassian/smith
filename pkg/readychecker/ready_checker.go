package readychecker

import (
	"log"
	"strings"

	"github.com/atlassian/smith"
	"github.com/atlassian/smith/pkg/resources"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	extensions "k8s.io/client-go/pkg/apis/extensions/v1beta1"
)

type IsObjectReady func(*unstructured.Unstructured) (isReady, retriableError bool, e error)

func alwaysReady(_ *unstructured.Unstructured) (isReady, retriableError bool, e error) {
	return true, false, nil
}

// each function is responsible for handling different versions of objects itself.
var knownTypes = map[schema.GroupKind]IsObjectReady{
	{Group: "", Kind: "ConfigMap"}:            alwaysReady,
	{Group: "", Kind: "Secret"}:               alwaysReady,
	{Group: "", Kind: "Service"}:              alwaysReady,
	{Group: "extensions", Kind: "Ingress"}:    alwaysReady,
	{Group: "extensions", Kind: "Deployment"}: isDeploymentReady,
}

// TprStore gets a TPR definition for a Group and Kind of the resource (TPR instance).
// Returns nil if TPR definition was not found.
type TprStore interface {
	Get(resource schema.GroupKind) (*extensions.ThirdPartyResource, error)
}

type ReadyChecker struct {
	Store TprStore
}

func (rc *ReadyChecker) IsReady(obj *unstructured.Unstructured) (isReady, retriableError bool, e error) {
	gk := obj.GroupVersionKind().GroupKind()

	// 1. Check if it is a known built-in resource
	if isObjectReady, ok := knownTypes[gk]; ok {
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
	actualValue := resources.GetNestedString(obj.Object, strings.Split(path, ".")...)
	if actualValue != value {
		// TODO this is for debugging, remove later
		log.Printf("[IsReady] %q is not equal to expected %q", actualValue, value)
		return false, false, nil
	}
	log.Printf("[IsReady] %q is equal to expected %q", actualValue, value)
	return true, false, nil
}
