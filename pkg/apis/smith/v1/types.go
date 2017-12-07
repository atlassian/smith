package v1

import (
	"bytes"
	"fmt"
	"reflect"

	"github.com/pkg/errors"
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

const (
	BundleReasonTerminalError  = "TerminalError"
	BundleReasonRetriableError = "RetriableError"
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
	BundleResourceSingular = "bundle"
	BundleResourcePlural   = "bundles"
	BundleResourceVersion  = "v1"
	BundleResourceKind     = "Bundle"

	BundleResourceGroupVersion = GroupName + "/" + BundleResourceVersion

	BundleResourceName = BundleResourcePlural + "." + GroupName
)

var BundleGVK = SchemeGroupVersion.WithKind(BundleResourceKind)

// +k8s:deepcopy-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type BundleList struct {
	meta_v1.TypeMeta `json:",inline"`
	// Standard list metadata.
	meta_v1.ListMeta `json:"metadata,omitempty"`

	// Items is a list of bundles.
	Items []Bundle `json:"items"`
}

// +genclient
// +genclient:noStatus

// +k8s:deepcopy-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
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
		cond.LastTransitionTime.Equal(&oldCondition.LastTransitionTime)

	if !isEqual {
		cond.LastUpdateTime = now
	}

	b.Status.Conditions[conditionIndex] = cond
	// Return true if one of the fields have changed.
	return !isEqual
}

// +k8s:deepcopy-gen=true
type BundleSpec struct {
	Resources []Resource `json:"resources"`
}

// +k8s:deepcopy-gen=true
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

// +k8s:deepcopy-gen=true
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

// +k8s:deepcopy-gen=true
type Resource struct {
	// Name of the resource for references.
	Name ResourceName `json:"name"`

	// Explicit dependencies.
	DependsOn []ResourceName `json:"dependsOn,omitempty"`

	// TODO nilebox: make "Normal" default type; there are no defaults for Bundle yet?
	Type ResourceType `json:"type"`

	Spec runtime.Object `json:"spec,omitempty"`

	PluginName string      `json:"pluginName,omitempty"`
	PluginSpec *PluginSpec `json:"pluginSpec,omitempty"`
}

type ResourceType string

const (
	Normal ResourceType = ""
	Plugin ResourceType = "plugin"
)

type PluginSpec struct {
	ApiVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
}

func (r *Resource) UnmarshalJSON(data []byte) error {
	var res struct {
		Name      ResourceName               `json:"name"`
		DependsOn []ResourceName             `json:"dependsOn"`
		Type      ResourceType               `json:"type"`
		Spec      *unstructured.Unstructured `json:"spec,omitempty"`

		PluginName string      `json:"pluginName,omitempty"`
		PluginSpec *PluginSpec `json:"pluginSpec,omitempty"`
	}
	err := json.Unmarshal(data, &res)
	if err != nil {
		return err
	}
	r.Name = res.Name
	r.DependsOn = res.DependsOn
	r.Type = res.Type
	r.Spec = res.Spec
	r.PluginName = res.PluginName
	r.PluginSpec = res.PluginSpec
	return nil
}

// IntoTyped tries to convert resource spec into a typed object passed as obj.
// It supports objects of the same type and Unstructured.
// Note that it does not perform a deep copy in case of typed API object.
// Note that this method may fail if references are used where a non-string value is expected.
func (r *Resource) IntoTyped(obj runtime.Object) error {
	if r.Type != Normal {
		return errors.Errorf("cannot convert non-Normal into typed (%s)", r.Type)
	}
	objT := reflect.TypeOf(r.Spec)
	if objT == reflect.TypeOf(obj) && objT.Kind() == reflect.Ptr {
		objV := reflect.ValueOf(obj)
		specV := reflect.ValueOf(r.Spec)

		objV.Elem().Set(specV.Elem()) // types are the same, dereference and assign value
		return nil
	}
	if specUnstr, ok := r.Spec.(*unstructured.Unstructured); ok {
		return unstructured_conversion.DefaultConverter.FromUnstructured(specUnstr.Object, obj)
	}
	return fmt.Errorf("cannot convert %T into typed object %T", r.Spec, obj)
}

func (r *Resource) Validate() error {
	switch r.Type {
	case Normal:
		if r.PluginName != "" || r.PluginSpec != nil {
			return errors.New("normal resource has plugin information")
		}
	case Plugin:
		if r.Spec != nil {
			return errors.New("plugin resource has normal information")
		}
	default:
		return errors.Errorf("invalid resource type %q", r.Type)
	}

	return nil
}

func (r *Resource) ObjectGVK() (schema.GroupVersionKind, error) {
	switch r.Type {
	case Normal:
		return r.Spec.GetObjectKind().GroupVersionKind(), nil
	case Plugin:
		if r.PluginSpec == nil {
			return schema.GroupVersionKind{}, errors.New("plugin specification is missing")
		}
		gv, err := schema.ParseGroupVersion(r.PluginSpec.ApiVersion)
		if err != nil {
			return schema.GroupVersionKind{}, errors.WithStack(err)
		}
		return gv.WithKind(r.PluginSpec.Kind), nil
	default:
		return schema.GroupVersionKind{}, errors.Errorf("invalid resource type %q", r.Type)
	}
}

func (r *Resource) ObjectName() (string, error) {
	switch r.Type {
	case Normal:
		m := r.Spec.(meta_v1.Object)
		return m.GetName(), nil
	case Plugin:
		if r.PluginSpec == nil {
			return "", errors.New("plugin specification is missing")
		}
		return r.PluginSpec.Name, nil
	default:
		return "", errors.Errorf("invalid resource type %q", r.Type)
	}
}
