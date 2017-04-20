package tprattribute

import (
	"encoding/json"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	TprDomain            = "tpr.atlassian.com"
	SleeperResourceGroup = TprDomain

	SleeperResourcePath         = "sleepers"
	SleeperResourceName         = "sleeper." + TprDomain
	SleeperResourceVersion      = "v1"
	SleeperResourceKind         = "Sleeper"
	SleeperResourceGroupVersion = SleeperResourceGroup + "/" + SleeperResourceVersion
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

func AddToScheme(scheme *runtime.Scheme) {
	scheme.AddKnownTypes(GV,
		&Sleeper{},
		&SleeperList{},
	)
	metav1.AddToGroupVersion(scheme, GV)
}

type SleeperList struct {
	metav1.TypeMeta `json:",inline"`
	// Standard list metadata.
	Metadata metav1.ListMeta `json:"metadata,omitempty"`

	// Items is a list of sleepers.
	Items []Sleeper `json:"items"`
}

// GetObjectKind is required to satisfy Object interface.
func (sl *SleeperList) GetObjectKind() schema.ObjectKind {
	return &sl.TypeMeta
}

// GetListMeta is required to satisfy ListMetaAccessor interface.
func (sl *SleeperList) GetListMeta() metav1.List {
	return &sl.Metadata
}

// Sleeper describes a sleeping resource.
type Sleeper struct {
	metav1.TypeMeta `json:",inline"`

	// Standard object metadata
	Metadata metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec is the specification of the desired behavior of the Sleeper.
	Spec SleeperSpec `json:"spec,omitempty"`

	// Status is most recently observed status of the Sleeper.
	Status SleeperStatus `json:"status,omitempty"`
}

// Required to satisfy Object interface
func (s *Sleeper) GetObjectKind() schema.ObjectKind {
	return &s.TypeMeta
}

// Required to satisfy ObjectMetaAccessor interface
func (s *Sleeper) GetObjectMeta() metav1.Object {
	return &s.Metadata
}

type SleeperSpec struct {
	SleepFor      int    `json:"sleepFor"`
	WakeupMessage string `json:"wakeupMessage"`
}

type SleeperStatus struct {
	State   SleeperState `json:"state,omitempty"`
	Message string       `json:"message,omitempty"`
}

// The code below is used only to work around a known problem with third-party
// resources and ugorji. If/when these issues are resolved, the code below
// should no longer be required.

type sleeperListCopy SleeperList
type sleeperCopy Sleeper

func (s *Sleeper) UnmarshalJSON(data []byte) error {
	tmp := sleeperCopy{}
	err := json.Unmarshal(data, &tmp)
	if err != nil {
		return err
	}
	*s = Sleeper(tmp)
	return nil
}

func (sl *SleeperList) UnmarshalJSON(data []byte) error {
	tmp := sleeperListCopy{}
	err := json.Unmarshal(data, &tmp)
	if err != nil {
		return err
	}
	*sl = SleeperList(tmp)
	return nil
}
