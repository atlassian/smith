package v1

import (
	"bytes"
	"reflect"

	"github.com/pkg/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	unstructured_conversion "k8s.io/apimachinery/pkg/conversion/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	k8s_json "k8s.io/apimachinery/pkg/util/json"
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

type ResourceConditionType string

// These are valid conditions of a resource.
const (
	ResourceBlocked    ResourceConditionType = "Blocked"
	ResourceInProgress ResourceConditionType = "InProgress"
	ResourceReady      ResourceConditionType = "Ready"
	ResourceError      ResourceConditionType = "Error"
)

const (
	// Blocked condition reasons

	ResourceReasonDependenciesNotReady = "DependenciesNotReady"
	ResourceReasonOtherResourceError   = "OtherResourceError"

	// Error condition reasons

	ResourceReasonTerminalError  = "TerminalError"
	ResourceReasonRetriableError = "RetriableError"
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
	for i := range b.Status.Conditions {
		condition := &b.Status.Conditions[i]
		if condition.Type == conditionType {
			return i, condition
		}
	}
	return -1, nil
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
// BundleStatus represents the latest available observations of a Bundle's current state.
type BundleStatus struct {
	Conditions       []BundleCondition `json:"conditions,omitempty"`
	ResourceStatuses []ResourceStatus  `json:"resourceStatuses,omitempty"`
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

func (bs *BundleStatus) GetResourceStatus(resName ResourceName) (int, *ResourceStatus) {
	for i := range bs.ResourceStatuses {
		resStatus := &bs.ResourceStatuses[i]
		if resStatus.Name == resName {
			return i, resStatus
		}
	}
	return -1, nil
}

// ResourceName is a reference to another Resource in the same bundle.
type ResourceName string

// PluginName is a name of a plugin to be invoked.
type PluginName string

// +k8s:deepcopy-gen=true
type Resource struct {
	// Name of the resource for references.
	Name ResourceName `json:"name"`

	// Explicit dependencies.
	DependsOn []ResourceName `json:"dependsOn,omitempty"`

	Spec ResourceSpec `json:"spec"`
}

// +k8s:deepcopy-gen=true
// ResourceSpec is a union type - either object of plugin can be specified.
type ResourceSpec struct {
	Object runtime.Object `json:"object,omitempty"`
	Plugin *PluginSpec    `json:"plugin,omitempty"`
}

// +k8s:deepcopy-gen=true
// PluginSpec holds the specification for a plugin.
type PluginSpec struct {
	Name       PluginName `json:"name"`
	ObjectName string     `json:"objectName"`
	// I feel like this should be json.RawMessage, but for reasons I don't properly
	// understand Service Catalog uses runtime.RawExtension for its Parameters (even
	// though they never get unpacked into a 'real' object?). So ...
	Spec runtime.RawExtension `json:"spec,omitempty"`
}

func (rs *ResourceSpec) UnmarshalJSON(data []byte) error {
	var res struct {
		Object *unstructured.Unstructured `json:"object,omitempty"`
		Plugin *PluginSpec                `json:"plugin,omitempty"`
	}
	err := k8s_json.Unmarshal(data, &res)
	if err != nil {
		return err
	}

	// If we blindly do this copy, we get nils with type info (i.e. != nil)
	if res.Object != nil {
		rs.Object = res.Object
	}

	rs.Plugin = res.Plugin
	return nil
}

// IntoTyped tries to convert resource spec into a typed object passed as obj.
// It supports objects of the same type and Unstructured.
// Note that it does not perform a deep copy in case of typed API object.
// Note that this method may fail if references are used where a non-string value is expected.
func (rs *ResourceSpec) IntoTyped(obj runtime.Object) error {
	if rs.Object == nil {
		return errors.New("cannot convert non-Object into typed")
	}
	objT := reflect.TypeOf(rs.Object)
	if objT == reflect.TypeOf(obj) && objT.Kind() == reflect.Ptr {
		objV := reflect.ValueOf(obj)
		specV := reflect.ValueOf(rs.Object)

		objV.Elem().Set(specV.Elem()) // types are the same, dereference and assign value
		return nil
	}
	if specUnstr, ok := rs.Object.(*unstructured.Unstructured); ok {
		return unstructured_conversion.DefaultConverter.FromUnstructured(specUnstr.Object, obj)
	}
	return errors.Errorf("cannot convert %T into typed object %T", rs.Object, obj)
}

// +k8s:deepcopy-gen=true
type ResourceStatus struct {
	Name       ResourceName        `json:"name"`
	Conditions []ResourceCondition `json:"conditions,omitempty"`
}

func (rs *ResourceStatus) GetCondition(conditionType ResourceConditionType) (int, *ResourceCondition) {
	for i := range rs.Conditions {
		resCond := &rs.Conditions[i]
		if resCond.Type == conditionType {
			return i, resCond
		}
	}
	return -1, nil
}

// +k8s:deepcopy-gen=true
// ResourceCondition describes the state of a resource at a certain point.
type ResourceCondition struct {
	// Type of Resource condition.
	Type ResourceConditionType `json:"type"`
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
