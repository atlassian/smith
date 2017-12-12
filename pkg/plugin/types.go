package plugin

// TODO refactor so we have more detail on plugin?
// (i.e. name, output types, so we can fail early if bundle is incorrect)
// OR have plugin validate itself

import (
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// NewFunc is a factory function that returns an initialized plugin.
// Called once on Smith startup.
type NewFunc func() (Plugin, error)

// Plugin represents a plugin and the functionality it provides.
type Plugin interface {
	// Describe returns information about the plugin.
	Describe() *Description
	// Process processes a plugin specification and produces an object as the result.
	Process(runtime.RawExtension, *Context) (*ProcessResult, error)
}

type Description struct {
	Name smith_v1.PluginName
	GVK  schema.GroupVersionKind
}

// Context contains contextual information for the Process() call.
type Context struct {
	Dependencies map[smith_v1.ResourceName]Dependency
}

// Dependency contains information about a dependency of a resource that a plugin is processing.
type Dependency struct {
	Spec      smith_v1.Resource
	Actual    runtime.Object
	Outputs   []runtime.Object
	Auxiliary []runtime.Object
}

// ProcessResult contains result of the Process() call.
type ProcessResult struct {
	Object runtime.Object
}
