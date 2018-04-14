package bundlec

import (
	"github.com/atlassian/smith"
	"github.com/atlassian/smith/pkg/resources"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	FinalizerDeleteResources = smith.Domain + "/deleteResources"
)

func hasDeleteResourcesFinalizer(accessor meta_v1.Object) bool {
	return resources.HasFinalizer(accessor, FinalizerDeleteResources)
}

func addDeleteResourcesFinalizer(finalizers []string) []string {
	return append(finalizers, FinalizerDeleteResources)
}

func removeDeleteResourcesFinalizer(finalizers []string) []string {
	newFinalizers := make([]string, 0, len(finalizers))
	for _, finalizer := range finalizers {
		if finalizer == FinalizerDeleteResources {
			continue
		}
		newFinalizers = append(newFinalizers, finalizer)
	}
	return newFinalizers
}
