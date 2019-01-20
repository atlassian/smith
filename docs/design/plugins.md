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
    references:
    - name: a-metadata-name
      resource: a
      path: metadata.name
    spec:
      object:
        apiVersion: servicecatalog.k8s.io/v1beta1
        kind: ServiceBinding
        metadata:
          name: a-binding
        spec:
          instanceRef:
            name: "!{a-metadata-name}"
          secretName: a-binding-secret

  - name: b
    references:
    - resource: a-binding
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
the plugin `foobar`. For each dependency (resources that are referenced in `references` attribute) of the
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
is known in advance. Smith validates that an object of the correct GVK is returned.
4. Have a name that is a [DNS_SUBDOMAIN](https://github.com/kubernetes/community/blob/master/contributors/design-proposals/architecture/identifiers.md)

A plugin does not need to set the name or the namespace of the returned object, it is set by Smith.

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
/*

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
*/

const  (
	PluginName smith_v1.PluginName = "filter"
)

func New() (smith_plugin.Plugin, error) {
	return &filterPlugin{}, nil
}

type filterPlugin struct {
}

func (p *filterPlugin) Process(spec map[string]interface{}, *smith_plugin.Context) smith_plugin.ProcessResult {
	// Possible error
	if isError {
		return &smith_plugin.ProcessResultFailure{
			//Error: error here,
			//IsExternalError: whether or not this is a user error,
			//IsRetriable: whether or not this error should be retried,
		}
	}

	// Do the processing
	return &smith_plugin.ProcessResultSuccess{
		//Object: object literal here
	}
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
    references:
    - name: a-metadata-name
      resource: a
      path: metadata.name
    spec:
      object:
        apiVersion: servicecatalog.k8s.io/v1beta1
        kind: ServiceBinding
        metadata:
          name: a-binding
        spec:
          instanceRef:
            name: "!{a-metadata-name}"
          secretName: a-binding-secret

  - name: b
    references:
    - resource: a-binding
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
