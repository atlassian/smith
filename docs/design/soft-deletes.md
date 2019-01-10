# Soft deletes

## Problem statement

In a declarative system lack of description of a resource means that it should not exist. So if it does,
it should be deleted. This is a potentially dangerous action that needs some safety around it to prevent irreversible
consequences deletion may have.

If resource is removed from a Bundle due to user error or a bug, it will be deleted once the Bundle reaches
the "ready" state. To reduce impact of such mistake it is beneficial to have some sort of "soft deletion" mechanism.

## Solution

Soft deletion is an opt-in feature based on resource definition in the Bundle enabled
via a special annotation `smith.a.c/deletionDelay`. This annotation will provide
a delay that the object should be kept in Kubernetes until proceeding with deletion.

When Smith detects an object in Kubernetes that is owned by a Bundle but is
missing from the resource list in the Bundle spec, it will check whether the object 
is marked with the `smith.a.c/deletionDelay` annotation. The annotation value is
a delay in the [Go duration format](https://golang.org/pkg/time/#ParseDuration).

If this annotation is present, Smith will annotate the corresponding object with
`smith.a.c/deletionTimestamp` with the current timestamp (`time.Now()`) as a value
to initiate the "countdown" of the delay, instead of issuing a delete immediately.
Then it will periodically keep checking if the deletion delay has expired.
Once it does, Smith will issue a delete request to Kubernetes API, thus **triggering
the actual deletion** of an object (this is a potentially irreversible action,
or "hard delete", so the deletion delay should be long enough for the user to be
able to cancel it, see below).

If the resource appears in the Bundle before the deletion delay has expired, Smith
will detect an existing "orphaned" resource, and will "adopt" it and remove the
`smith.a.c/deletionTimestamp` annotation, thus cancelling the deletion countdown.
It will also continue updating "actual" object spec to make it match the "desired"
spec declared in the Bundle. In other words, the normal processing of the resource
will continue without actual deletion.

## Example

```yaml
apiVersion: smith.atlassian.com/v1
kind: Bundle
metadata:
  name: my-bundle
  namespace: my-ns
spec:
  resources:
  - name: my-db
    spec:
      object:
        apiVersion: servicecatalog.k8s.io/v1beta1
        kind: ServiceInstance
        metadata:
          name: db
          annotations:
            # 24-hour delay before proceeding with deletion
            smith.atlassian.com/deletionDelay: 24h
        spec:
          clusterServiceClassExternalID: 59c9355a-92d4-4c07-a633-96f6fa51abf1
          clusterServicePlanExternalID: 7b002afa-4197-4a50-a41a-df2dec4b5cfa
          parameters:
            ...
```

## Forced "hard delete"

User may want to force the "hard delete" (i.e. the actual deletion of Kubernetes object)
instead of waiting for the deletion delay to expire. To do that, currently user needs to
manually issue a delete request of the underlying Kubernetes object, e.g. using a
corresponding `kubectl delete` command.
