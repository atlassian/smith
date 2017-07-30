package tprattribute

import (
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	CrdDomain            = "crd.atlassian.com"
	SleeperResourceGroup = CrdDomain

	SleeperResourceSingular = "sleeper"
	SleeperResourcePlural   = "sleepers"
	SleeperResourceVersion  = "v1"
	SleeperResourceKind     = "Sleeper"

	SleeperResourceGroupVersion = SleeperResourceGroup + "/" + SleeperResourceVersion

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

var GV = schema.GroupVersion{
	Group:   SleeperResourceGroup,
	Version: SleeperResourceVersion,
}

var SleeperGVK = GV.WithKind(SleeperResourceKind)

func AddToScheme(scheme *runtime.Scheme) {
	scheme.AddKnownTypes(GV,
		&Sleeper{},
		&SleeperList{},
	)
	meta_v1.AddToGroupVersion(scheme, GV)
}

type SleeperList struct {
	meta_v1.TypeMeta `json:",inline"`
	// Standard list metadata.
	meta_v1.ListMeta `json:"metadata,omitempty"`

	// Items is a list of sleepers.
	Items []Sleeper `json:"items"`
}

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

type SleeperSpec struct {
	SleepFor      int    `json:"sleepFor"`
	WakeupMessage string `json:"wakeupMessage"`
}

type SleeperStatus struct {
	State   SleeperState `json:"state,omitempty"`
	Message string       `json:"message,omitempty"`
}
