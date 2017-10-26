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
      apiVersion: servicecatalog.k8s.io/v1beta1
      kind: ServiceInstance
      metadata:
        name: a
      spec:
        foo: bar

  - name: a-binding
    dependsOn:
    - a
    spec:
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
    - a
    spec:
      apiVersion: servicecatalog.k8s.io/v1beta1
      kind: ServiceInstance
      metadata:
        name: b
      spec:
        # need data from a-binding-secret but transformed
        # e.g. only keys which start with "FOO_" when full names of the keys are not known in advance.
```

## Specification

Smith plugins are [Go plugins](https://golang.org/pkg/plugin/). They are eagerly loaded at Smith startup to detect
issues early. Names of plugins to load are passed via command line.
Each plugin publishes a `Process(smith_v1.Resource, map[smith_v1.ResourceName]Dependency) (TransformResult, error)`
function.

When Smith comes across a resource with `type: plugin` and `pluginName: foobar` it invokes
the plugin `foobar`. To do that it inspects all the dependencies of the resource specified in `dependsOn` attribute
and fetches their outputs to include in the invocation along with the dependencies themselves. Smith needs to
recognize resource group/version/kinds to be able to fetch the output (if there is one).
One example is `ServiceBinding` that produces a `Secret`.

A resource must have the group/version/kind of the object that is going to be produced specified.

## Plugin skeleton:

```go
package main

import (
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	smith_plugin "github.com/atlassian/smith/pkg/plugin"
)

// For reference:
//
//type Dependency struct {
//	Spec    smith_v1.Resource
//	Actual  runtime.Object
//	Outputs []runtime.Object
//}
//
//type ProcessResult struct {
//	Object runtime.Object
//}

func Process(resource smith_v1.Resource, dependencies map[smith_v1.ResourceName]smith_plugin.Dependency)  (smith_plugin.ProcessResult, error) {
	// Do the processing
	return smith_plugin.ProcessResult{
		//Object: object literal here
	}, nil
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
      apiVersion: servicecatalog.k8s.io/v1beta1
      kind: ServiceInstance
      metadata:
        name: a
      spec:
        foo: bar

  - name: a-binding
    dependsOn:
    - a
    spec:
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
    type: plugin
    pluginName: filter
    pluginSpec:
      apiVersion: servicecatalog.k8s.io/v1beta1
      kind: ServiceInstance
      name: b
      spec:
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
    name: a
    key: FOO_BAR1
  - secretKeyRef:
    name: a
    key: FOO_BAR2
```

## Glossary

- resource - Each resource is either an object definition or a plugin
invocation definition. `Bundle` contains a list of resources.
