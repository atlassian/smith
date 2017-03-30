package smith

import (
	"encoding/json"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	apiv1 "k8s.io/client-go/pkg/api/v1"
)

type BundleConditionType string

// These are valid conditions of a Bundle.
const (
	BundleInProgress BundleConditionType = "InProgress"
	BundleReady      BundleConditionType = "Ready"
	BundleError      BundleConditionType = "Error"
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

type ByNameStore interface {
	Get(gvk schema.GroupVersionKind, namespace, name string) (obj runtime.Object, exists bool, err error)
}

type BundleList struct {
	metav1.TypeMeta `json:",inline"`
	// Standard list metadata.
	Metadata metav1.ListMeta `json:"metadata,omitempty"`

	// Items is a list of bundles.
	Items []Bundle `json:"items"`
}

// GetObjectKind is required to satisfy Object interface.
func (bl *BundleList) GetObjectKind() schema.ObjectKind {
	return &bl.TypeMeta
}

// GetListMeta is required to satisfy ListMetaAccessor interface.
func (bl *BundleList) GetListMeta() metav1.List {
	return &bl.Metadata
}

// Bundle describes a resources bundle.
type Bundle struct {
	metav1.TypeMeta `json:",inline"`

	// Standard object metadata
	Metadata metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec is the specification of the desired behavior of the Bundle.
	Spec BundleSpec `json:"spec,omitempty"`

	// Status is most recently observed status of the Bundle.
	Status BundleStatus `json:"status,omitempty"`
}

// Required to satisfy Object interface
func (b *Bundle) GetObjectKind() schema.ObjectKind {
	return &b.TypeMeta
}

// Required to satisfy ObjectMetaAccessor interface
func (b *Bundle) GetObjectMeta() metav1.Object {
	return &b.Metadata
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
	now := metav1.Now()
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
	// Status of the condition, one of True, False, Unknown.
	Status apiv1.ConditionStatus `json:"status"`
	// The last time this condition was updated.
	LastUpdateTime metav1.Time `json:"lastUpdateTime,omitempty"`
	// Last time the condition transitioned from one status to another.
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`
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

// The code below is used only to work around a known problem with third-party
// resources and ugorji. If/when these issues are resolved, the code below
// should no longer be required.

type bundleListCopy BundleList
type bundleCopy Bundle

func (b *Bundle) UnmarshalJSON(data []byte) error {
	tmp := bundleCopy{}
	err := json.Unmarshal(data, &tmp)
	if err != nil {
		return err
	}
	*b = Bundle(tmp)
	return nil
}

func (bl *BundleList) UnmarshalJSON(data []byte) error {
	tmp := bundleListCopy{}
	err := json.Unmarshal(data, &tmp)
	if err != nil {
		return err
	}
	*bl = BundleList(tmp)
	return nil
}
