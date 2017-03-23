# Managing resources

This file describes the way Smith manages instances of Third Party Resources (TPR) and other resources.

## Field references (not implemented)

Cross-object field references are a way for an object to get output field(s) of its dependency
(or multiple dependencies) injected into it as a field (or multiple fields). References to only direct
dependencies are supported.

Syntax `"$({dependency1.fieldName})"` means that value of `fieldName` will be injected as a string instead
of the placeholder. In this case value must be a string, boolean or number.

Syntax `"$(({dependency1.fieldName}))"` means that value of `fieldName` will be injected without quotes
instead of the placeholder. In this case it can be a of any type including objects.

This syntax is selected to be as close to
[Templates proposal](https://github.com/kubernetes/community/blob/master/contributors/design-proposals/templates.md)
as possible so that it can be used in the future to implement substitution.

```
apiVersion: smith.atlassian.com/v1
kind: Bundle
metadata:
  name: bundlex
spec:
  resources:
  - name: sleeper1
    spec:
      apiVersion: tpr.atlassian.com/v1
      kind: Sleeper
      metadata:
        name: sleeper1
      spec:
        sleepFor: 3
        wakeupMessage: Hello, Infravators!
        
--- comes later
      status:
        message: "Hello, Infravators!"
  - name: sleeper2
    dependsOn:
    - sleeper1
    spec:
      apiVersion: tpr.atlassian.com/v1
      kind: Sleeper
      metadata:
        name: sleeper2
      spec:
        sleepFor: 4
        wakeupMessage: "$(sleeper1.status.message)"
  - name: sleeper3
    dependsOn:
    - sleeper2
    spec:
      apiVersion: tpr.atlassian.com/v1
      kind: Sleeper
      metadata:
        name: sleeper3
      spec: "$((sleeper2.spec))"
```

## Defined annotations

### smith.a.c/TprReadyWhenFieldPath=`<FieldPath>`, smith.a.c/TprReadyWhenFieldValue=`<Value>`

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
    smith.atlassian.com/TprReadyWhenFieldPath: status.state
    smith.atlassian.com/TprReadyWhenFieldValue: Ready
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

## Defined but not implemented

### smith.a.c/TprReadyWhenExistsKind=`<Kind>`, smith.a.c/TprReadyWhenExistsVersion=`<GroupVersion>`

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
    smith.atlassian.com/TprReadyWhenExistsKind: ResourceBinding
    smith.atlassian.com/TprReadyWhenExistsVersion: smith.atlassian.com/v1
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

### smith.a.c/TprOutputNameModeField=`<NameModeField>`, smith.a.c/TprOutputNameSetterField=`<OutputNameSetterField>`, smith.a.c/TprOutputNameField=`<OutputNameField>`

These two annotations work together with `smith.a.c/TprReadyWhenExistsKind` and `smith.a.c/TprReadyWhenExistsVersion`.
Please refer to their definition for definitions of `Tinst` and `K`.

`smith.a.c/TprOutputNameModeField` can be applied to a TPR `T` to configure the way name of the output object `O`
is constructed. Object `O` is a [Secret](https://kubernetes.io/docs/user-guide/secrets/) or a
[ConfigMap](https://kubernetes.io/docs/user-guide/configmap/) which holds parameters that may be injected into
object(s) `D` depending on `Tinst`. Object `O` is created/updated before `K` is created/updated.

The following values of `smith.a.c/TprOutputNameModeField` are defined:
- `Random` - `O`'s name is randomly generated on each `O` update.
- `ContentsHash` - `O`'s name is a deterministic hash function of `O`'s contents. `O`'s name changes if, and only if, it's contents change.
- `Fixed` - `O`'s name is provided via a field `<OutputNameSetterField>` on `Tinst`. Name of that field is defined by
`smith.a.c/TprOutputNameSetterField` set on `T`.

Each time `O`'s name changes a new `O` object is created, all dependent objects will have references updated and the old object will be deleted.

Smith does not use `smith.a.c/TprOutputNameModeField` and `smith.a.c/TprOutputNameSetterField` annotations directly,
they are defined here for completeness only. It is the job of a TPR controller to honor/support them and it is up to
whoever constructs the Template object with that TPR to use those fields or not.

`smith.a.c/TprOutputNameField` is the name of a field on `K` that contains the actual name of `O` that has been created.

Example of a TPR `T`:

```
apiVersion: extensions/v1beta1
description: Smith example Resource Claim
kind: ThirdPartyResource
metadata:
  name: resource-claim.smith.atlassian.com
  annotations:
    smith.atlassian.com/TprReadyWhenExistsKind: ResourceBinding
    smith.atlassian.com/TprReadyWhenExistsVersion: smith.atlassian.com/v1
    smith.atlassian.com/TprOutputNameModeField: spec.secretNameMode
    smith.atlassian.com/TprOutputNameSetterField: spec.secretName
    smith.atlassian.com/TprOutputNameField: status.secretName
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
spec:
  secretNameMode: ContentsHash
  
  # Could be:
  # secretNameMode: Fixed
  # secretName: my-secret
```

Object `K`:

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
status:
  secretName: 231864817236487163 # hash of contents
  
  # For secretNameMode: Fixed:
  # secretName: my-secret
```

Object `O`:

```
apiVersion: v1
kind: Secret
metadata:
  name: 231864817236487163

  # For secretNameMode: Fixed:
  # name: my-secret
data:
  username: admin
  password: godsexlove
type: Opaque
```
