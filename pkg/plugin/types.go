package plugin

// TODO refactor so we have more detail on plugin?
// (i.e. name, output types, so we can fail early if bundle is incorrect)
// OR have plugin validate itself

import (
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"

	"k8s.io/apimachinery/pkg/runtime"
)

const (
	FuncName = "Process"
)

type Func func(resource smith_v1.Resource, dependencies map[smith_v1.ResourceName]Dependency) (ProcessResult, error)

type Dependency struct {
	Spec      smith_v1.Resource
	Actual    runtime.Object
	Outputs   []runtime.Object
	Auxiliary []runtime.Object
}

type ProcessResult struct {
	Object runtime.Object
}
