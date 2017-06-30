package smith

import (
	"bytes"

	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	unstructured_conversion "k8s.io/apimachinery/pkg/conversion/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/json"
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
	cond := *condition // copy to avoid mutating the original
	now := meta_v1.Now()
	cond.LastTransitionTime = now
	// Try to find this bundle condition.
	conditionIndex, oldCondition := b.GetCondition(cond.Type)

	if oldCondition == nil {
		// We are adding new bundle condition.
		b.Status.Conditions = append(b.Status.Conditions, cond)
		return true
	}
	// We are updating an existing condition, so we need to check if it has changed.
	if cond.Status == oldCondition.Status {
		cond.LastTransitionTime = oldCondition.LastTransitionTime
	}

	isEqual := cond.Status == oldCondition.Status &&
		cond.Reason == oldCondition.Reason &&
		cond.Message == oldCondition.Message &&
		cond.LastTransitionTime.Equal(oldCondition.LastTransitionTime)

	if !isEqual {
		cond.LastUpdateTime = now
	}

	b.Status.Conditions[conditionIndex] = cond
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

func (bs *BundleStatus) ShortString() string {
	first := true
	var buf bytes.Buffer
	buf.WriteByte('[')
	for _, cond := range bs.Conditions {
		if first {
			first = false
		} else {
			buf.WriteByte('|')
		}
		buf.WriteString(string(cond.Type))
		buf.WriteByte(' ')
		buf.WriteString(string(cond.Status))
		if cond.Reason != "" {
			buf.WriteByte(' ')
			buf.WriteByte('"')
			buf.WriteString(cond.Reason)
			buf.WriteByte('"')
		}
		if cond.Message != "" {
			buf.WriteByte(' ')
			buf.WriteByte('"')
			buf.WriteString(cond.Message)
			buf.WriteByte('"')
		}
	}
	buf.WriteByte(']')
	return buf.String()
}

// ResourceName is a reference to another Resource in the same bundle.
type ResourceName string

type Resource struct {
	// Name of the resource for references.
	Name ResourceName `json:"name"`

	// Explicit dependencies.
	DependsOn []ResourceName `json:"dependsOn,omitempty"`

	Spec runtime.Object `json:"spec"`
}

// ToUnstructured returns Spec field as an Unstructured object.
// It makes a copy if it is an Unstructured already.
func (r *Resource) ToUnstructured(copy DeepCopy) (*unstructured.Unstructured, error) {
	if _, ok := r.Spec.(*unstructured.Unstructured); ok {
		uCopy, err := copy(r.Spec)
		if err != nil {
			return nil, err
		}
		return uCopy.(*unstructured.Unstructured), nil
	}
	u := &unstructured.Unstructured{
		Object: make(map[string]interface{}),
	}
	err := unstructured_conversion.DefaultConverter.ToUnstructured(r.Spec, &u.Object)
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (r *Resource) UnmarshalJSON(data []byte) error {
	type resource struct {
		Name      ResourceName              `json:"name"`
		DependsOn []ResourceName            `json:"dependsOn"`
		Spec      unstructured.Unstructured `json:"spec"`
	}
	var res resource
	err := json.Unmarshal(data, &res)
	if err != nil {
		return err
	}
	r.Name = res.Name
	r.DependsOn = res.DependsOn
	r.Spec = &res.Spec
	return nil
}
