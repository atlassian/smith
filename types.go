package smith

import (
	"encoding/json"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type ResourceState string

const (
	NEW            ResourceState = ""
	IN_PROGRESS    ResourceState = "InProgress"
	READY          ResourceState = "Ready"
	ERROR          ResourceState = "Error"
	TERMINAL_ERROR ResourceState = "TerminalError"
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
func (tl *BundleList) GetObjectKind() schema.ObjectKind {
	return &tl.TypeMeta
}

// GetListMeta is required to satisfy ListMetaAccessor interface.
func (tl *BundleList) GetListMeta() metav1.List {
	return &tl.Metadata
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
func (t *Bundle) GetObjectKind() schema.ObjectKind {
	return &t.TypeMeta
}

// Required to satisfy ObjectMetaAccessor interface
func (t *Bundle) GetObjectMeta() metav1.Object {
	return &t.Metadata
}

type BundleSpec struct {
	Resources []Resource `json:"resources"`
}

type BundleStatus struct {
	State ResourceState `json:"state,omitempty"`
}

// DependencyRef is a reference to another Resource in the same bundle.
type DependencyRef string

type Resource struct {
	// Name of the resource for references.
	Name string `json:"name"`

	// Explicit dependencies.
	DependsOn []DependencyRef `json:"dependsOn,omitempty"`

	Spec unstructured.Unstructured `json:"spec"`
}

// The code below is used only to work around a known problem with third-party
// resources and ugorji. If/when these issues are resolved, the code below
// should no longer be required.

type bundleListCopy BundleList
type bundleCopy Bundle

func (e *Bundle) UnmarshalJSON(data []byte) error {
	tmp := bundleCopy{}
	err := json.Unmarshal(data, &tmp)
	if err != nil {
		return err
	}
	*e = Bundle(tmp)
	return nil
}

func (el *BundleList) UnmarshalJSON(data []byte) error {
	tmp := bundleListCopy{}
	err := json.Unmarshal(data, &tmp)
	if err != nil {
		return err
	}
	*el = BundleList(tmp)
	return nil
}
