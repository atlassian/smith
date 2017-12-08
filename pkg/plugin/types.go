package plugin

// TODO refactor so we have more detail on plugin?
// (i.e. name, output types, so we can fail early if bundle is incorrect)
// OR have plugin validate itself

import (
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"

	"k8s.io/apimachinery/pkg/runtime"
)

type NewFunc func() (Plugin, error)

type Plugin interface {
	Describe() *Description
	Process(*smith_v1.Resource, *Context) (*ProcessResult, error)
}

type Description struct {
	Name smith_v1.PluginName
}

type Context struct {
	Dependencies map[smith_v1.ResourceName]Dependency
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
