# Smith

**Smith** is a Kubernetes workflow engine / resource manager **prototype**.
It's not complete yet, it's still under active development.
It may or may not fulfil the requirements of https://github.com/kubernetes/kubernetes/issues/1704.

## The idea

What if we build a service that allows us to manage Kubernetes' built-in resources and other
[Third Party Resources](https://github.com/kubernetes/kubernetes/blob/master/docs/design/extending-api.md)
(TPR) in a generic way? Similar to how AWS CloudFormation (or Google Deployment Manager) allows us to manage any
AWS/GCE and custom resource. Then we could expose all the resources we need
to integrate as Third Party Resources and manage them declaratively. This is an open architecture
with Kubernetes as its core. Other microservices can create/update/watch TPRs to co-ordinate their work/lifecycle.

## Implementation

A group of resources is defined using a Bundle (just like a Stack for AWS CloudFormation).
The Bundle itself is also a Kubernetes TPR.
Smith watches for new instances of a Bundle (and events to existing ones), picks them up and processes them.

Processing involves parsing the bundle, building a dependency graph (which is implicitly defined in the bundle),
walking the graph, and creating/updating necessary resources. Each created/referenced resource gets
an label pointing at the origin Bundle.

### Example bundle
TPR definitions:
```yaml
apiVersion: extensions/v1beta1
kind: ThirdPartyResource
description: "Resource bundle definition"
metadata:
  name: bundle.smith.atlassian.com
versions:
  - name: v1
```
```yaml
apiVersion: extensions/v1beta1
kind: ThirdPartyResource
description: "Postgresql resource definition"
metadata:
  name: postgresql-resource.smith-sql.atlassian.com # must use another group due to https://github.com/kubernetes/kubernetes/issues/23831
versions:
  - name: v1
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
      apiVersion: smith-sql.atlassian.com/v1
      kind: PostgresqlResource
      metadata:
        name: db1
      spec:
        disk: 100GiB
  - name: app1
    dependsOn:
    - db1
    spec:
      apiVersion: extensions/v1beta1
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

Resources can reference outputs of other resources within the same bundle. [See what is supported](./docs/design/managing-resources.md). 

### Dependencies
Resources may depend on each other explicitly via `DependsOn` object references. Resources are created in the reverse dependency order.

### States
READY is the state of a Resource when it can be considered created. E.g. if it is
a DB then it means it was provisioned and set up as requested. State is often part of Status but it depends on kind of resource.

### Event-driven and stateless
Smith does not block while waiting for a resource to reach the READY state. Instead, when walking the dependency
graph, if a resource is not in the READY state (still being created) it skips processing of that resource.
Of course resources that don't have their dependencies READY are not processed either.
Resources that can be created concurrently are created concurrently.
Full bundle re-processing is triggered by events about the watched resources.
Smith is watching all supported resource types and inspects labels on events to find out which
bundle should be re-processed because of the event. This scales better than watching
individual resources and much better than polling individual resources.

## Notes

### On [App Controller](https://github.com/Mirantis/k8s-AppController)
Mirantis App Controller (discussed here https://github.com/kubernetes/kubernetes/issues/29453) is a very similar workflow engine with a few differences.

1. Graph of dependencies is defined explicitly.
2. It uses polling and blocks while waiting for the resource to become READY.
3. The goal of Smith is to manage instances of TPRs. App Controller cannot manage them as of this writing.

It is not better or worse, just different set of design choices.

### Requirements

* Please run on Kubernetes 1.4+ because earlier versions have some bugs that may prevent Smith from working properly;
* Go 1.7+ is required because [context package](https://golang.org/doc/go1.7#context) is used and it was added to
standard library in this version;
* Working Docker installation - build process uses dockerized Go to isolate from the host system;
* List of project dependencies and their versions can be found in `glide.yaml` and `glide.lock` files.

### Building

* To install dependencies run
```bash
make setup-ci
```
* To run integration tests with [minikube](https://github.com/kubernetes/minikube) run
```bash
make minikube-test
```
* To run against minikube run
```bash
make minikube-run
```
* To build the docker image run
```bash
make docker
# or make docker-race to build a binary with -race
```

### Contributing

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

# License

Copyright (c) 2016-2017 Atlassian and others. Apache 2.0 licensed, see LICENSE file.
