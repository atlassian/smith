package plugin

// TODO refactor so we have more detail on plugin?
// (i.e. name, output types, so we can fail early if bundle is incorrect)
// OR have plugin validate itself

import (
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type ProcessResultType string

const (
	ProcessResultSuccessType ProcessResultType = "Success"
	ProcessResultFailureType ProcessResultType = "Error"
)

// NewFunc is a factory function that returns an initialized plugin.
// Called once on Smith startup.
type NewFunc func() (Plugin, error)

// Plugin represents a plugin and the functionality it provides.
type Plugin interface {
	// Describe returns information about the plugin.
	Describe() *Description
	// Process processes a plugin specification and produces an object as the result.
	Process(map[string]interface{}, *Context) ProcessResult
}

type Description struct {
	Name smith_v1.PluginName
	GVK  schema.GroupVersionKind
	// gojsonschema supported schema for the spec (first argument of Process)
	SpecSchema []byte
}

// Context contains contextual information for the Process() call.
type Context struct {
	// Namespace is the namespace where the returned object will be created.
	Namespace string
	// Actual is the actual object that will be updated if it exists already.
	// nil if the object does not exist.
	Actual runtime.Object
	// Dependencies is the map from dependency name to a description of that dependency.
	Dependencies map[smith_v1.ResourceName]Dependency
}

// Dependency contains information about a dependency of a resource that a plugin is processing.
type Dependency struct {
	// Spec is the specification of the resource as specified in the Bundle.
	Spec smith_v1.Resource
	// Actual is the actual dependency object.
	Actual runtime.Object
	// Outputs are objects produced by the actual object.
	Outputs []runtime.Object
	// Auxiliary are objects that somehow relate to the actual object.
	Auxiliary []runtime.Object
}

// ProcessResult contains result of the Process() call.
type ProcessResult interface {
	StatusType() ProcessResultType
}

type ProcessResultSuccess struct {
	// Object is the object that should be created/updated.
	Object runtime.Object
}

type ProcessResultFailure struct {
	Error            error
	IsExternalError  bool
	IsRetriableError bool
}

func (r *ProcessResultSuccess) StatusType() ProcessResultType {
	return ProcessResultSuccessType
}

func (r *ProcessResultFailure) StatusType() ProcessResultType {
	return ProcessResultFailureType
}
