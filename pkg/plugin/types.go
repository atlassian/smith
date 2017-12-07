package plugin

// TODO refactor so we have more detail on plugin?
// (i.e. name, output types, so we can fail early if bundle is incorrect)
// OR have plugin validate itself

import (
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"

	"k8s.io/apimachinery/pkg/runtime"
)

const (
	ProcessFuncName     = "Process"
	IsSupportedFuncName = "IsSupported"
)

type Process func(resource smith_v1.Resource, dependencies map[smith_v1.ResourceName]Dependency) (ProcessResult, error)
type IsSupported func(plugin string) (bool, error)

type Plugin struct {
	Name        string
	Process     Process
	IsSupported IsSupported
}

type Dependency struct {
	Spec      smith_v1.Resource
	Actual    runtime.Object
	Outputs   []runtime.Object
	Auxiliary []runtime.Object
}

type ProcessResult struct {
	Object runtime.Object
}
