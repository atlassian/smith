package bundlec

import (
	"github.com/atlassian/smith"

	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	FinalizerDeleteResources string = smith.Domain + "/deleteResources"
)

func HasFinalizer(accessor meta_v1.Object, finalizer string) bool {
	finalizers := accessor.GetFinalizers()
	for _, f := range finalizers {
		if f == finalizer {
			return true
		}
	}
	return false
}

func hasDeleteResourcesFinalizer(accessor meta_v1.Object) bool {
	return HasFinalizer(accessor, FinalizerDeleteResources)
}

func addDeleteResourcesFinalizerToObject(accessor meta_v1.Object) {
	accessor.SetFinalizers(addDeleteResourcesFinalizer(accessor.GetFinalizers()))
}

func addDeleteResourcesFinalizer(finalizers []string) []string {
	return append(finalizers, FinalizerDeleteResources)
}

func removeDeleteResourcesFinalizerFromObject(accessor meta_v1.Object) {
	accessor.SetFinalizers(removeDeleteResourcesFinalizer(accessor.GetFinalizers()))
}

func removeDeleteResourcesFinalizer(finalizers []string) []string {
	newFinalizers := []string{}
	for _, finalizer := range finalizers {
		if finalizer == FinalizerDeleteResources {
			continue
		}
		newFinalizers = append(newFinalizers, finalizer)
	}
	return newFinalizers
}
