package v1

import (
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	CrdDomain = "crd.atlassian.com"

	SleeperResourceSingular = "sleeper"
	SleeperResourcePlural   = "sleepers"
	SleeperResourceVersion  = "v1"
	SleeperResourceKind     = "Sleeper"

	SleeperResourceGroupVersion = GroupName + "/" + SleeperResourceVersion

	SleeperResourceName = SleeperResourcePlural + "." + CrdDomain

	SleeperReadyStatePath  = "{$.status.state}"
	SleeperReadyStateValue = Awake
)

type SleeperState string

const (
	New      SleeperState = ""
	Sleeping SleeperState = "Sleeping"
	Awake    SleeperState = "Awake!"
	Error    SleeperState = "Error"
)

var SleeperGVK = SchemeGroupVersion.WithKind(SleeperResourceKind)

// +k8s:deepcopy-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type SleeperList struct {
	meta_v1.TypeMeta `json:",inline"`
	// Standard list metadata.
	meta_v1.ListMeta `json:"metadata,omitempty"`

	// Items is a list of sleepers.
	Items []Sleeper `json:"items"`
}

// +k8s:deepcopy-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// Sleeper describes a sleeping resource.
type Sleeper struct {
	meta_v1.TypeMeta `json:",inline"`

	// Standard object metadata
	meta_v1.ObjectMeta `json:"metadata,omitempty"`

	// Spec is the specification of the desired behavior of the Sleeper.
	Spec SleeperSpec `json:"spec,omitempty"`

	// Status is most recently observed status of the Sleeper.
	Status SleeperStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen=true
type SleeperSpec struct {
	SleepFor      int    `json:"sleepFor"`
	WakeupMessage string `json:"wakeupMessage"`
}

// +k8s:deepcopy-gen=true
type SleeperStatus struct {
	State   SleeperState `json:"state,omitempty"`
	Message string       `json:"message,omitempty"`
}
