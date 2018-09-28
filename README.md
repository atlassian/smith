# Smith

[![Godoc](https://godoc.org/github.com/atlassian/smith?status.svg)](https://godoc.org/github.com/atlassian/smith)
[![Build Status](https://travis-ci.org/atlassian/smith.svg?branch=master)](https://travis-ci.org/atlassian/smith)
[![Coverage Status](https://coveralls.io/repos/github/atlassian/smith/badge.svg?branch=master)](https://coveralls.io/github/atlassian/smith?branch=master)
[![GitHub tag](https://img.shields.io/github/tag/atlassian/smith.svg?maxAge=86400)](https://github.com/atlassian/smith)
[![Docker Pulls](https://img.shields.io/docker/pulls/atlassianlabs/smith.svg)](https://hub.docker.com/r/atlassianlabs/smith/)
[![Docker Stars](https://img.shields.io/docker/stars/atlassianlabs/smith.svg)](https://hub.docker.com/r/atlassianlabs/smith/)
[![MicroBadger Layers Size](https://images.microbadger.com/badges/image/atlassianlabs/smith.svg)](https://microbadger.com/images/atlassianlabs/smith)
[![Go Report Card](https://goreportcard.com/badge/github.com/atlassian/smith)](https://goreportcard.com/report/github.com/atlassian/smith)
[![license](https://img.shields.io/github/license/atlassian/smith.svg)](LICENSE)

**Smith** is a Kubernetes workflow engine / resource manager.

It's functional and under active development.

## News

- 01.01.2018: [Milestone v1.0](https://github.com/atlassian/smith/milestones/v1.0) is complete and v1.0.0 released!

## The idea

What if we build a service that allows us to manage Kubernetes' built-in resources and other
[Custom Resources](https://kubernetes.io/docs/concepts/api-extension/custom-resources/) (CRs) in a generic way?
Similar to how AWS CloudFormation (or Google Deployment Manager) allows us to manage any
AWS/GCE and custom resource. Then we could expose all the resources we need
to integrate as Custom Resources and manage them declaratively. This is an open architecture
with Kubernetes as its core. Other controllers can create/update/watch CRs to co-ordinate their work/lifecycle.

## Implementation

A group of resources is defined using a Bundle (just like a Stack for AWS CloudFormation).
The Bundle itself is also a Kubernetes CR.
Smith watches for new instances of a Bundle (and events to existing ones), picks them up and processes them.

Processing involves parsing the bundle, building a dependency graph (which is implicitly defined in the bundle),
walking the graph, and creating/updating necessary resources. Each created/referenced resource gets
a controller owner reference pointing at the origin Bundle.

### Example bundle
CR definitions:

For `Bundle` see [0-crd.yaml](docs/deployment/0-crd.yaml).
```yaml
apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  name: postgresql-resources.smith.atlassian.com
spec:
  group: smith.atlassian.com
  version: v1
  names:
    kind: PostgresqlResource
    plural: postgresqlresources
    singular: postgresqlresource
```
Bundle:
```yaml
apiVersion: smith.atlassian.com/v1
kind: Bundle
description: "Sample resource bundle"
metadata:
  name: bundle1
spec:
  resources:
  - name: db1
    spec:
      object:
        apiVersion: smith.atlassian.com/v1
        kind: PostgresqlResource
        metadata:
          name: db1
        spec:
          disk: 100GiB
  - name: app1
    references:
    - resource: db1
    spec:
      object:
        apiVersion: apps/v1
        kind: Deployment
        metadata:
          name: app1
        spec:
          replicas: 1
          bundle:
            metadata:
              labels:
                app: app1
            spec:
              containers:
              - name: app1
                image: quay.io/some/app1
```

### Outputs
Some resource types can have Outputs:
- Values placed into Status field of the object
- New objects likes [Secrets](https://kubernetes.io/docs/user-guide/secrets/) and/or [ConfigMaps](https://kubernetes.io/docs/user-guide/configmap/)
- [Service Catalog](https://github.com/kubernetes-incubator/service-catalog) [objects](https://github.com/kubernetes-incubator/service-catalog/blob/master/docs/v1/api.md)

Resources can reference outputs of other resources within the same bundle. [See what is supported](./docs/design/field-references.md).

### Dependencies
Resources may depend on each other explicitly via object references. Resources are created in the reverse dependency order.

### States
READY is the state of a Resource when it can be considered created. E.g. if it is
a DB then it means it was provisioned and set up as requested. State is often part of Status but it depends on kind of resource.

### Event-driven and stateless
Smith does not block while waiting for a resource to reach the READY state. Instead, when walking the dependency
graph, if a resource is not in the READY state (still being created) it skips processing of that resource.
Resources that don't have their dependencies READY are not processed.
Resources that can be created concurrently are created concurrently.
Full bundle re-processing is triggered by events about the watched resources.
Smith is watching all supported resource kinds and reacts to events to determine which bundle should be re-processed.
This scales better than watching individual resources and much better than polling individual resources.
Smith controller is built according to [recommendations](https://github.com/kubernetes/community/blob/master/contributors/devel/controllers.md)
and following the same behaviour, semantics and code "style" as native Kubernetes controllers as closely as possible.

## Features

- Supported object kinds: `Deployment`, `Service`, `ConfigMap`, `Secret`, `Ingress`;
- [Service Catalog](https://github.com/kubernetes-incubator/service-catalog) support: objects with kind `ServiceInstance` and `ServiceBinding`.
See [an example](examples/service_catalog) and
[recording of the presentation](https://youtu.be/7fgPgtQh5Es) to [Service Catalog SIG](https://github.com/kubernetes/community/tree/master/sig-service-catalog);
- Dynamic Custom Resources support via [special annotations](docs/design/managing-resources.md#defined-annotations);
- References between objects in the graph to pull parts of objects/fields from dependencies;
- Smith will delete objects which were removed from a Bundle when Bundle reconciliation is performed (e.g. on a Bundle update);
- [Plugins](docs/design/plugins.md) framework for injecting custom behavior when walking the dependency graph;

## Notes

### Presentations
Smith has been presented to:
- SIG Service Catalog - see information, screencast and recording [here](examples/service_catalog).
- SIG Apps - see [recoding of the meeting](https://youtu.be/Eak9EN1PVds?t=875).

### On [App Controller](https://github.com/Mirantis/k8s-AppController)
Mirantis App Controller (discussed here https://github.com/kubernetes/kubernetes/issues/29453) is a very similar workflow engine with a few differences.

* Graph of dependencies is defined explicitly.
* It uses polling and blocks while waiting for the resource to become READY.
* The goal of Smith is to manage Custom Resources and Service Catalog objects. App Controller cannot manage them as of this writing (?).
* Smith has very advanced support for Service Catalog objects.

### On [Helm](https://helm.sh/)
Helm is a package manager for Kubernetes. Smith operates on a lower level, even though it can be used by a human,
that is not the main use case. Smith is built to be used as a foundation component with human-friendly tooling built
on top of it. E.g. Helm could probably use Smith under the covers to manipulate Kubernetes API objects. Another
use case is a PaaS that delegates (some) object manipulations to Smith.

## Requirements

* Kubernetes 1.11+ is required - we use
[`/status` subresource](https://kubernetes.io/docs/tasks/access-kubernetes-api/custom-resources/custom-resource-definitions/#status-subresource)
and OpenAPI schema features that became available in this version; 
* List of project dependencies and their versions can be found in `Gopkg.toml` and `Gopkg.lock` files.

## Building, testing and running

* [`dep`](https://github.com/golang/dep) is used for package management. Please [install it](https://github.com/golang/dep#setup).
* [`Bazel`](https://www.bazel.build/) is used as the build tool. Please [install it](https://docs.bazel.build/versions/master/install.html).
* To install dependencies run
```bash
make setup
```

Integration tests can be run against any Kuberentes context that is configured locally. To see which contexts are
available run:
```bash
kubectl config get-contexts
```
By default a context named `minikube` is used. If you use [minikube](https://github.com/kubernetes/minikube) and want
to run tests against that context then you don't need to do anything extra. If you want to run against some other
context you may do so by setting the `KUBE_CONTEXT` environment variable which is honored by the makefile.

E.g. to run against Kubernetes-for-Docker use `KUBE_CONTEXT=docker-for-desktop`.

* To run integration tests run
```bash
make integration-test
```
* To run integration tests for [Service Catalog](https://github.com/kubernetes-incubator/service-catalog) support run
```bash
make integration-test-sc
```
This command assumes Service Catalog and UPS Broker are installed in the cluster. To install them follow the
[Service Catalog walkthrough](https://github.com/kubernetes-incubator/service-catalog/blob/master/docs/walkthrough.md).
* To run Smith locally against the configured context run
```bash
make run
# or to run with Service Catalog support enabled
make run-sc
```
* To build the Docker image run
```bash
make docker
```
This command only builds the image, which is not very useful. If you want to import it into your Docker run
```bash
make docker-export
```

## Contributing

Pull requests, issues and comments welcome. For pull requests:

* Add tests for new features and bug fixes
* Follow the existing style
* Separate unrelated changes into multiple pull requests

See the existing issues for things to start contributing.

For bigger changes, make sure you start a discussion first by creating an issue and explaining the intended change.

Atlassian requires contributors to sign a Contributor License Agreement, known as a CLA. This serves as a record
stating that the contributor is entitled to contribute the code/documentation/translation to the project and is willing
to have it used in distributions and derivative works (or is willing to transfer ownership).

Prior to accepting your contributions we ask that you please follow the appropriate link below to digitally sign the
CLA. The Corporate CLA is for those who are contributing as a member of an organization and the individual CLA is for
those contributing as an individual.

* [CLA for corporate contributors](https://na2.docusign.net/Member/PowerFormSigning.aspx?PowerFormId=e1c17c66-ca4d-4aab-a953-2c231af4a20b)
* [CLA for individuals](https://na2.docusign.net/Member/PowerFormSigning.aspx?PowerFormId=3f94fbdc-2fbe-46ac-b14c-5d152700ae5d)

## Stargazers over time

[![Stargazers over time](https://starcharts.herokuapp.com/atlassian/smith.svg)](https://starcharts.herokuapp.com/atlassian/smith)

## License

Copyright (c) 2016-2018 Atlassian and others. Apache 2.0 licensed, see LICENSE file.
