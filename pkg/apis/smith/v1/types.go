package v1

import (
	"bytes"
	"fmt"

	"github.com/atlassian/smith/pkg/apis/smith"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
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

type PluginStatusStr string

const (
	PluginStatusOk           PluginStatusStr = "Ok"
	PluginStatusNoSuchPlugin PluginStatusStr = "NoSuchPlugin"
)

const (
	BundleResourceSingular = "bundle"
	BundleResourcePlural   = "bundles"
	BundleResourceVersion  = "v1"
	BundleResourceKind     = "Bundle"

	BundleResourceGroupVersion = smith.GroupName + "/" + BundleResourceVersion

	BundleResourceName = BundleResourcePlural + "." + smith.GroupName

	ReferenceModifierBindSecret = "bindsecret"
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
	Resources []Resource `json:"resources,omitempty"`
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

func (bc *BundleCondition) String() string {
	var buf bytes.Buffer
	buf.WriteString(string(bc.Type))   // nolint: gosec
	buf.WriteByte(' ')                 // nolint: gosec
	buf.WriteString(string(bc.Status)) // nolint: gosec
	if bc.Reason != "" {
		fmt.Fprintf(&buf, " %q", bc.Reason) // nolint: errcheck
	}
	if bc.Message != "" {
		fmt.Fprintf(&buf, " %q", bc.Message) // nolint: errcheck
	}
	return buf.String()
}

type PluginStatus struct {
	Name    PluginName      `json:"name"`
	Group   string          `json:"group"`
	Version string          `json:"version"`
	Kind    string          `json:"kind"`
	Status  PluginStatusStr `json:"status,omitempty"`
}

// +k8s:deepcopy-gen=true
// BundleStatus represents the latest available observations of a Bundle's current state.
type BundleStatus struct {
	Conditions       []BundleCondition `json:"conditions,omitempty"`
	ResourceStatuses []ResourceStatus  `json:"resourceStatuses,omitempty"`
	ObjectsToDelete  []ObjectToDelete  `json:"objectsToDelete,omitempty"`
	PluginStatuses   []PluginStatus    `json:"pluginStatuses,omitempty"`
}

func (bs *BundleStatus) String() string {
	first := true
	var buf bytes.Buffer
	buf.WriteByte('[') // nolint: gosec
	for _, cond := range bs.Conditions {
		if first {
			first = false
		} else {
			buf.WriteByte('|') // nolint: gosec
		}
		buf.WriteString(cond.String()) // nolint: gosec
	}
	buf.WriteByte(']') // nolint: gosec
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

// ReferenceName is a the name of a reference which can be used inside a resource.
type ReferenceName string

// PluginName is a name of a plugin to be invoked.
type PluginName string

// +k8s:deepcopy-gen=true
// Resource describes an object that should be provisioned.
type Resource struct {
	// Name of the resource for references.
	Name ResourceName `json:"name"`

	// Explicit dependencies.
	References []Reference `json:"references,omitempty"`

	Spec ResourceSpec `json:"spec"`
}

// +k8s:deepcopy-gen=true
// Refer to a part of another object
type Reference struct {
	Name     ReferenceName `json:"name,omitempty"`
	Resource ResourceName  `json:"resource"`
	Path     string        `json:"path,omitempty"`
	Example  interface{}   `json:"example,omitempty"`
	Modifier string        `json:"modifier,omitempty"`
}

// DeepCopyInto is an deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Reference) DeepCopyInto(out *Reference) {
	*out = *in
	out.Example = runtime.DeepCopyJSONValue(in.Example)
}

// Ref returns string representation of the reference that can be used to pull in the referred entity.
func (in *Reference) Ref() string {
	return "!{" + string(in.Name) + "}"
}

// +k8s:deepcopy-gen=true
// ResourceSpec is a union type - either object of plugin can be specified.
type ResourceSpec struct {
	Object runtime.Object `json:"object,omitempty"`
	Plugin *PluginSpec    `json:"plugin,omitempty"`
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

// +k8s:deepcopy-gen=true
// PluginSpec holds the specification for a plugin.
type PluginSpec struct {
	Name       PluginName             `json:"name"`
	ObjectName string                 `json:"objectName"`
	Spec       map[string]interface{} `json:"spec,omitempty"`
}

// DeepCopyInto is an deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PluginSpec) DeepCopyInto(out *PluginSpec) {
	*out = *in
	out.Spec = runtime.DeepCopyJSON(in.Spec)
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

type ObjectToDelete struct {
	// GVK of the object.

	Group   string `json:"group"`
	Version string `json:"version"`
	Kind    string `json:"kind"`
	// Name of the object.
	Name string `json:"name"`
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

func (rc *ResourceCondition) String() string {
	var buf bytes.Buffer
	buf.WriteString(string(rc.Type))   // nolint: gosec
	buf.WriteByte(' ')                 // nolint: gosec
	buf.WriteString(string(rc.Status)) // nolint: gosec
	if rc.Reason != "" {
		fmt.Fprintf(&buf, " %q", rc.Reason) // nolint: errcheck
	}
	if rc.Message != "" {
		fmt.Fprintf(&buf, " %q", rc.Message) // nolint: errcheck
	}
	return buf.String()
}
