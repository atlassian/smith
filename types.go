package smith

import (
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type BundleConditionType string

// These are valid conditions of a Bundle.
const (
	BundleInProgress BundleConditionType = "InProgress"
	BundleReady      BundleConditionType = "Ready"
	BundleError      BundleConditionType = "Error"
)

type ConditionStatus string

// These are valid condition statuses. "ConditionTrue" means a resource is in the condition.
// "ConditionFalse" means a resource is not in the condition. "ConditionUnknown" means kubernetes
// can't decide if a resource is in the condition or not. In the future, we could add other
// intermediate conditions, e.g. ConditionDegraded.
const (
	ConditionTrue    ConditionStatus = "True"
	ConditionFalse   ConditionStatus = "False"
	ConditionUnknown ConditionStatus = "Unknown"
)

const (
	SmithDomain        = "smith.atlassian.com"
	SmithResourceGroup = SmithDomain

	BundleResourcePath         = "bundles"
	BundleResourceName         = "bundle." + SmithDomain
	BundleResourceVersion      = "v1"
	BundleResourceKind         = "Bundle"
	BundleResourceGroupVersion = SmithResourceGroup + "/" + BundleResourceVersion

	BundleNameLabel = BundleResourceName + "/BundleName"

	// See docs/design/managing-resources.md
	TprFieldPathAnnotation  = SmithDomain + "/TprReadyWhenFieldPath"
	TprFieldValueAnnotation = SmithDomain + "/TprReadyWhenFieldValue"
)

var GV = schema.GroupVersion{
	Group:   SmithResourceGroup,
	Version: BundleResourceVersion,
}

var BundleGVK = GV.WithKind(BundleResourceKind)

type DeepCopy func(src interface{}) (interface{}, error)

type ByNameStore interface {
	Get(gvk schema.GroupVersionKind, namespace, name string) (obj runtime.Object, exists bool, err error)
}

func AddToScheme(scheme *runtime.Scheme) {
	scheme.AddKnownTypes(GV,
		&Bundle{},
		&BundleList{},
	)
	meta_v1.AddToGroupVersion(scheme, GV)
}

type BundleList struct {
	meta_v1.TypeMeta `json:",inline"`
	// Standard list metadata.
	meta_v1.ListMeta `json:"metadata,omitempty"`

	// Items is a list of bundles.
	Items []Bundle `json:"items"`
}

// Bundle describes a resources bundle.
type Bundle struct {
	meta_v1.TypeMeta `json:",inline"`

	// Standard object metadata
	meta_v1.ObjectMeta `json:"metadata,omitempty"`

	// Spec is the specification of the desired behavior of the Bundle.
	Spec BundleSpec `json:"spec,omitempty"`

	// Status is most recently observed status of the Bundle.
	Status BundleStatus `json:"status,omitempty"`
}

func (b *Bundle) GetCondition(conditionType BundleConditionType) (int, *BundleCondition) {
	for i, condition := range b.Status.Conditions {
		if condition.Type == conditionType {
			return i, &condition
		}
	}
	return -1, nil
}

// Updates existing Bundle condition or creates a new one. Sets LastTransitionTime to now if the
// status has changed.
// Returns true if Bundle condition has changed or has been added.
func (b *Bundle) UpdateCondition(condition *BundleCondition) bool {
	now := meta_v1.Now()
	condition.LastTransitionTime = now
	// Try to find this bundle condition.
	conditionIndex, oldCondition := b.GetCondition(condition.Type)

	if oldCondition == nil {
		// We are adding new bundle condition.
		b.Status.Conditions = append(b.Status.Conditions, *condition)
		return true
	}
	// We are updating an existing condition, so we need to check if it has changed.
	if condition.Status == oldCondition.Status {
		condition.LastTransitionTime = oldCondition.LastTransitionTime
	}

	isEqual := condition.Status == oldCondition.Status &&
		condition.Reason == oldCondition.Reason &&
		condition.Message == oldCondition.Message &&
		condition.LastTransitionTime.Equal(oldCondition.LastTransitionTime)

	if !isEqual {
		condition.LastUpdateTime = now
	}

	b.Status.Conditions[conditionIndex] = *condition
	// Return true if one of the fields have changed.
	return !isEqual

}

type BundleSpec struct {
	Resources []Resource `json:"resources"`
}

// BundleCondition describes the state of a bundle at a certain point.
type BundleCondition struct {
	// Type of Bundle condition.
	Type BundleConditionType `json:"type"`
	// Status of the condition.
	Status ConditionStatus `json:"status"`
	// The last time this condition was updated.
	LastUpdateTime meta_v1.Time `json:"lastUpdateTime,omitempty"`
	// Last time the condition transitioned from one status to another.
	LastTransitionTime meta_v1.Time `json:"lastTransitionTime,omitempty"`
	// The reason for the condition's last transition.
	Reason string `json:"reason,omitempty"`
	// A human readable message indicating details about the transition.
	Message string `json:"message,omitempty"`
}

type BundleStatus struct {
	// Represents the latest available observations of a Bundle's current state.
	Conditions []BundleCondition `json:"conditions,omitempty"`
}

// ResourceName is a reference to another Resource in the same bundle.
type ResourceName string

type Resource struct {
	// Name of the resource for references.
	Name ResourceName `json:"name"`

	// Explicit dependencies.
	DependsOn []ResourceName `json:"dependsOn,omitempty"`

	Spec unstructured.Unstructured `json:"spec"`
}
