# Plugins

This file describes a plugin mechanism for Smith. A plugin can be used to transform
dependencies and their outputs into specifications of objects to be created.

## Motivation

A typical example when a runtime transformation is needed is two `ServiceInstance`s, one producing inputs for the
other. Say `b` depends on `a`. Quite often the shape of data `ServiceInstance` `a` produces does not match what
`ServiceInstance` `b` expects.

```yaml
apiVersion: smith.atlassian.com/v1
kind: Bundle
metadata:
  name: ab
spec:
  resources:

  - name: a
    spec:
      object:
        apiVersion: servicecatalog.k8s.io/v1beta1
        kind: ServiceInstance
        metadata:
          name: a
        spec:
          clusterServiceClassExternalName: user-provided-service
          clusterServicePlanExternalName: default
          parameters:
            credentials:
              foo: bar

  - name: a-binding
    dependsOn:
    - a
    spec:
      object:
        apiVersion: servicecatalog.k8s.io/v1beta1
        kind: ServiceBinding
        metadata:
          name: a-binding
        spec:
          instanceRef:
            name: "{{a#metadata.name}}"
          secretName: a-binding-secret

  - name: b
    dependsOn:
    - a-binding
    spec:
      object:
        apiVersion: servicecatalog.k8s.io/v1beta1
        kind: ServiceInstance
        metadata:
          name: b
        spec:
          # need data from a-binding-secret but transformed
          # e.g. only keys which start with "FOO_" when full names of the keys are not known in advance.
```

## Specification

Smith plugins are a way to extend Smith with functionality to do additional runtime processing.
Plugins are eagerly loaded at Smith startup to detect issues early.
Each plugin publishes a factory function `New() (Plugin, error)` that is called once upon Smith startup to
get an instance of a plugin.

See types in `pkg/plugin` for more details.

When Smith comes across a resource with `spec.plugin` field set and `spec.plugin.name: foobar` it invokes
the plugin `foobar`. For each dependency (resources that are referenced in `dependsOn` attribute) of the
resource with plugin invocation Smith fetches its output objects (if any) and auxiliary objects (if any) to
include in the plugin invocation along with the dependencies themselves.
Smith needs to recognize resource group/version/kinds to be able to fetch the outputs and auxiliary objects.
One example is `ServiceBinding` that produces a `Secret` (output object) and references a `ServiceInstance`
(an auxiliary object).

A plugin must:
1. Be a pure function - plugin must not depend on any external state;
2. Be deterministic - same set of inputs should always produce identical output:
  - no unordered data structures;
  - no unstable sort algorithms;
  - no timestamps.
3. Output an object of the correct Group/Version/Kind - GVK is declared in the plugin resource definition and
is known in advance.

## Plugin skeleton

```go
package main

import (
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	smith_plugin "github.com/atlassian/smith/pkg/plugin"
	
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// For reference:
//
//type Dependency struct {
//	Spec      smith_v1.Resource
//	Actual    runtime.Object
//	Outputs   []runtime.Object
//	Auxiliary []runtime.Object
//}
//
//type ProcessResult struct {
//	Object runtime.Object
//}

const  (
	PluginName smith_v1.PluginName = "filter"
)

func New() (smith_plugin.Plugin, error) {
	return &filterPlugin{}, nil
}

type filterPlugin struct {
}

func (p *filterPlugin) Process(spec runtime.RawExtension, *smith_plugin.Context) (*smith_plugin.ProcessResult, error) {
	// Do the processing
	return &smith_plugin.ProcessResult{
		//Object: object literal here
	}, nil
}

func (p *filterPlugin) Describe() *smith_plugin.Description {
	return &smith_plugin.Description{
		Name: PluginName,
		GVK: schema.GroupVersionKind{
			Group: "servicecatalog.k8s.io/v1beta1",
			Version: "v1beta1",
			Kind: "ServiceInstance",
		},
	}
}
```

## Example

```yaml
apiVersion: smith.atlassian.com/v1
kind: Bundle
metadata:
  name: ab
spec:
  resources:

  - name: a
    spec:
      object:
        apiVersion: servicecatalog.k8s.io/v1beta1
        kind: ServiceInstance
        metadata:
          name: a
        spec:
          clusterServiceClassExternalName: user-provided-service
          clusterServicePlanExternalName: default
          parameters:
            credentials:
              foo: bar

  - name: a-binding
    dependsOn:
    - a
    spec:
      object:
        apiVersion: servicecatalog.k8s.io/v1beta1
        kind: ServiceBinding
        metadata:
          name: a-binding
        spec:
          instanceRef:
            name: "{{a#metadata.name}}"
          secretName: a-binding-secret

  - name: b
    dependsOn:
    - a-binding
    spec:
      plugin:
        name: filter
        spec:
          name: b
          filterByPrefix: "FOO_" # only keys which start with "FOO_"
```

When the plugin `filter` is invoked, it returns the following object:

```yaml
apiVersion: servicecatalog.k8s.io/v1beta1
kind: ServiceInstance
name: b
spec:
  parametersFrom:
  - secretKeyRef:
    name: a-binding-secret
    key: FOO_BAR1
  - secretKeyRef:
    name: a-binding-secret
    key: FOO_BAR2
```

## Glossary

- resource - Each resource is either an object definition or a plugin
invocation definition. `Bundle` contains a list of resources.
