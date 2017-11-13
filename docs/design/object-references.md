# External object references

An external object reference can be used to reference an object that is not part of the Bundle being processed.

## Motivation

When using [plugins](plugins.md), sometimes it is necessary to invoke the plugin with an object that is not
part of the Bundle being processed. A reference can be used to make Smith fetch an external object and
pass it to the plugin.

## Specification

References can only reference objects inside of the same namespace (namespaced references) or
non-namespaced objects (cluster references). 

### Namespaced references

To reference an object in the same namespace, its name, group, version and kind are needed. Example:

```yaml
apiVersion: smith.atlassian.com/v1
kind: Bundle
metadata:
  name: b1
  namespace: namespace123
spec:
  resources:

  - name: extra-ref
    type: reference
    referenceSpec:
      apiVersion: rbac.authorization.k8s.io/v1
      kind: RoleBinding
      name: binding1
```

### Cluster references

To reference a non-namespaced object, its name, group, version and kind are needed. Example:

```yaml
apiVersion: smith.atlassian.com/v1
kind: Bundle
metadata:
  name: b2
  namespace: namespace123
spec:
  resources:

  - name: cluster-extra-ref
    type: clusterReference
    clusterReferenceSpec:
      apiVersion: rbac.authorization.k8s.io/v1
      kind: ClusterRoleBinding
      name: binding2
```
