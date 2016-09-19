# What it is

**Smith** is a Kubernetes workflow engine / resource manager **prototype**. It's not complete yet, under development.
It may or may not fulfil https://github.com/kubernetes/kubernetes/issues/1704.

## The idea

What if we build a service that allows us to manage Kubernetes' built-in resources and other
[Third Party Resources](https://github.com/kubernetes/kubernetes/blob/master/docs/design/extending-api.md)
(TPR) in a generic way like AWS CloudFormation (or Google Deployment Manager) allows to manage any
AWS/GCE resources and custom resources? Then we could expose all resources we need
to integrate as Third Party Resources and manage them declaratively. This is an open architecture
with Kubernetes as its core. Other microservices create/update/watch TPRs to co-ordinate their work/lifecycle.

## Implementation

Bunch of resources is defined using a Template (just like Stack for AWS CloudFormation).
Template itself is defined as a Kubernetes TPR.
Smith watches for new instances of Template (and events to existing ones), picks them up and processes them.

Processing is parsing the template, building a dependency graph implicitly defined in the template
and walking the graph, creating/updating necessary resources.

### Outputs
Some resource types can have Outputs - some values that are produced by creation of the resource in/by a
third party system. For example a TPR may define a DB to be created. Outputs will be name of the Secret
object with the password to access the DB, a plain string with username, DB URL and so on.
Outputs may be consumed by other resources by referencing them.

### Dependencies
Resources may depend on each other explicitly via DependsOn object references or implicitly
via references to other resources' outputs. Resources should be created in the reverse dependency order. 

### States
READY is a state of a Resource indicating that the resource can be considered created. E.g. if it is
a DB that it mean it was provisioned and set up as requested. State is part of
[Status](https://github.com/kubernetes/kubernetes/blob/master/docs/devel/api-conventions.md#spec-and-status).

### Event-driven and stateless
Smith does not block, waiting for a resource to reach the READY state. Instead, when walking the dependency
graph, when a resource is created/updated (if not in the READY state already) it stops processing the
template. Full template re-processing is triggered by events about the watched resources. Smith is
watching all supported resource types and inspects annotations on events to find out which
template should be re-processed because of this event. This should scale better than watching
individual resources and much better than polling individual resources.

## Notes

### On [App Controller](https://github.com/Mirantis/k8s-AppController)
Mirantis App Controller (discussed here https://github.com/kubernetes/kubernetes/issues/29453) is a very similar thing with a few differences.

1. Presumably it works :)
2. Graph of dependencies is defined explicitly. IMHO this is not the best user experience.
3. It uses polling and blocks while waiting for the resource to become READY.
4. The goal of Smith is to manage instances of TPRs. App Controller cannot manage them yet.

It is not better or worse, just different set of design choices.
