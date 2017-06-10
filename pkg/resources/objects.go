package resources

import (
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GetControllerOf returns the controllerRef if controllee has a controller, otherwise returns nil.
func GetControllerOf(controllee meta_v1.Object) *meta_v1.OwnerReference {
	for _, ref := range controllee.GetOwnerReferences() {
		if ref.Controller != nil && *ref.Controller {
			return &ref
		}
	}
	return nil
}
