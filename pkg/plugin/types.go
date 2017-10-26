package plugin

import (
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"

	"k8s.io/apimachinery/pkg/runtime"
)

type Func func(resource smith_v1.Resource, dependencies map[smith_v1.ResourceName]Dependency) (ProcessResult, error)

type Dependency struct {
	Spec    smith_v1.Resource
	Actual  runtime.Object
	Outputs []runtime.Object
}

type ProcessResult struct {
	Object runtime.Object
}
