package tprattribute

import (
	"encoding/json"

	"k8s.io/client-go/pkg/api/meta"
	apiv1 "k8s.io/client-go/pkg/api/v1"
	metav1 "k8s.io/client-go/pkg/apis/meta/v1"
	"k8s.io/client-go/pkg/runtime/schema"
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
	NEW      SleeperState = ""
	SLEEPING SleeperState = "Sleeping"
	AWAKE    SleeperState = "Awake!"
	ERROR    SleeperState = "Error"
)

type SleeperList struct {
	metav1.TypeMeta `json:",inline"`
	// Standard list metadata.
	Metadata metav1.ListMeta `json:"metadata,omitempty"`

	// Items is a list of sleepers.
	Items []Sleeper `json:"items"`
}

// GetObjectKind is required to satisfy Object interface.
func (tl *SleeperList) GetObjectKind() schema.ObjectKind {
	return &tl.TypeMeta
}

// GetListMeta is required to satisfy ListMetaAccessor interface.
func (tl *SleeperList) GetListMeta() metav1.List {
	return &tl.Metadata
}

// Sleeper describes a sleeping resource.
type Sleeper struct {
	metav1.TypeMeta `json:",inline"`

	// Standard object metadata
	Metadata apiv1.ObjectMeta `json:"metadata,omitempty"`

	// Spec is the specification of the desired behavior of the Sleeper.
	Spec SleeperSpec `json:"spec,omitempty"`

	// Status is most recently observed status of the Sleeper.
	Status SleeperStatus `json:"status,omitempty"`
}

// Required to satisfy Object interface
func (t *Sleeper) GetObjectKind() schema.ObjectKind {
	return &t.TypeMeta
}

// Required to satisfy ObjectMetaAccessor interface
func (t *Sleeper) GetObjectMeta() meta.Object {
	return &t.Metadata
}

type SleeperSpec struct {
	SleepFor      int    `json:"sleepFor"`
	WakeupMessage string `json:"wakeupMessage"`
}

type SleeperStatus struct {
	State   SleeperState `json:"state,omitempty"`
	Message string
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
	tmp2 := Sleeper(tmp)
	*s = tmp2
	return nil
}

func (sl *SleeperList) UnmarshalJSON(data []byte) error {
	tmp := sleeperListCopy{}
	err := json.Unmarshal(data, &tmp)
	if err != nil {
		return err
	}
	tmp2 := SleeperList(tmp)
	*sl = tmp2
	return nil
}
