# Managing resources

This file describes the way Smith manages instances of Third Party Resources (TPR) and other resources.

## Defined annotations

### smith.a.c/tpr/readyWhenExistsKind=`<Kind>`, smith.a.c/tpr/readyWhenExistsVersion=`<GroupVersion>`

Applied to a TPR `T` to indicate that an instance of it `Tinst` is considered `READY` when a resource of
Kind=`<Kind>` `K` exists in the same namespace and that resource has an
[Owner Reference](https://kubernetes.io/docs/api-reference/v1.5/#ownerreference-v1) pointing to `Tinst`.

Example of a TPR `T`:

```
apiVersion: extensions/v1beta1
description: Smith example Resource Claim
kind: ThirdPartyResource
metadata:
  name: resource-claim.smith.atlassian.com
  annotations:
    smith.atlassian.com/tpr/readyWhenExistsKind: ResourceBinding
    smith.atlassian.com/tpr/readyWhenExistsVersion: smith.atlassian.com/v1
versions:
- name: v1

```

Example of a TPR instance `Tinst`:

```
apiVersion: smith.atlassian.com/v1
kind: ResourceClaim
metadata:
  name: DbClaim1
  uid: 038b49a6-e746-11e6-baf3-ee8d75af8f6e
```

TPR instance `Tinst` will be considered `READY` when the following object `K` is created:

```
apiVersion: smith.atlassian.com/v1
kind: ResourceBinding
metadata:
  name: BindingForDbClaim1
  ownerReferences:
  - apiVersion: smith.atlassian.com/v1
    kind: ResourceClaim
    name: DbClaim1
    uid: 038b49a6-e746-11e6-baf3-ee8d75af8f6e
```

### smith.a.c/tpr/readyWhenFieldPath=`<FieldPath>`, smith.a.c/tpr/readyWhenFieldValue=`<Value>`

Applied to a TPR `T` to indicate that an instance of it `Tinst` is considered `READY` when it has a field,
located by `<FieldPath>`, that equals `<Value>`.

Example of a TPR `T`:

```
apiVersion: extensions/v1beta1
description: Smith example AWS CloudFormation stack
kind: ThirdPartyResource
metadata:
  name: cloud-formation.smith.atlassian.com
  annotations:
    smith.atlassian.com/tpr/readyWhenFieldPath: status.state
    smith.atlassian.com/tpr/readyWhenFieldValue: Ready
versions:
- name: v1

```

Example of a TPR instance `Tinst`:

```
apiVersion: smith.atlassian.com/v1
kind: CloudFormation
metadata:
  name: cfn-1
spec:
  ...
status:
  state: Ready
```
