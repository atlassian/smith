# Managing resources

This file describes the way Smith manages
[Custom Resources](https://kubernetes.io/docs/concepts/api-extension/custom-resources/) (CRs) and other resources.

## Field references

Cross-object field references are a way for an object to get output field(s) of its dependency
(or multiple dependencies) injected into it as a field (or multiple fields). References to only direct
dependencies are supported.

Syntax `"{{dependency1#fieldName}}"` means that value of `fieldName` will be injected as a string instead
of the placeholder. In this case value must be a string, boolean or number.

Syntax `"{{{dependency1#fieldName}}}"` means that value of `fieldName` will be injected without quotes
instead of the placeholder. In this case it can be of any type including objects.

The `fieldName` could be specified in JsonPath format (with `$.` prefix added by default), for example:
`{{dependency1#status.conditions[?(@.type=="Ready")].status}}`

```yaml
apiVersion: smith.atlassian.com/v1
kind: Bundle
metadata:
  name: bundlex
spec:
  resources:
  - name: sleeper1
    spec:
      apiVersion: crd.atlassian.com/v1
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
      apiVersion: crd.atlassian.com/v1
      kind: Sleeper
      metadata:
        name: sleeper2
      spec:
        sleepFor: 4
        wakeupMessage: "{{sleeper1#status.message}}"
  - name: sleeper3
    dependsOn:
    - sleeper2
    spec:
      apiVersion: crd.atlassian.com/v1
      kind: Sleeper
      metadata:
        name: sleeper3
      spec: "{{{sleeper2#spec}}}"
```

## Defined annotations

### smith.a.c/CrdReadyWhenFieldPath=`<FieldPath>`, smith.a.c/CrdReadyWhenFieldValue=`<Value>`

Applied to a CRD `T` to indicate that an instance of it `Tinst` is considered `READY` when it has a field,
located by `<FieldPath>`, that equals `<Value>`. The `<FieldPath>` value must be specified in
[JsonPath](http://goessner.net/articles/JsonPath/) format.

Example of a CRD `T`:

```yaml
apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  name: cloud-formations.smith.atlassian.com
  annotations:
    smith.atlassian.com/CrReadyWhenFieldPath: "{{$.status.state}}"
    smith.atlassian.com/CrReadyWhenFieldValue: Ready
spec:
  group: smith.atlassian.com
  version: v1
  names:
    kind: CloudFormation
    plural: cloudformations
    singular: cloudformation
```

Example of a CR `Tinst`:

```yaml
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

### smith.a.c/CrReadyWhenExistsKind=`<Kind>`, smith.a.c/CrReadyWhenExistsVersion=`<GroupVersion>`

Applied to a CRD `T` to indicate that an instance of it `Tinst` is considered `READY` when a resource of
Kind=`<Kind>` `K` exists in the same namespace and that resource has an
[Owner Reference](https://kubernetes.io/docs/api-reference/v1.5/#ownerreference-v1) pointing to `Tinst`.

Example of a CRD `T`:

```yaml
apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  name: resource-claims.smith.atlassian.com
  annotations:
    smith.atlassian.com/CrReadyWhenExistsKind: ResourceBinding
    smith.atlassian.com/CrReadyWhenExistsVersion: smith.atlassian.com/v1
spec:
  group: smith.atlassian.com
  version: v1
  names:
    kind: ResourceClaim
    plural: resourceclaims
    singular: resourceclaim
```

Example of a CR `Tinst`:

```yaml
apiVersion: smith.atlassian.com/v1
kind: ResourceClaim
metadata:
  name: DbClaim1
  uid: 038b49a6-e746-11e6-baf3-ee8d75af8f6e
```

CR `Tinst` will be considered `READY` when the following object `K` is created:

```yaml
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

### smith.a.c/CrOutputNameModeField=`<NameModeField>`, smith.a.c/CrOutputNameSetterField=`<OutputNameSetterField>`, smith.a.c/CrOutputNameField=`<OutputNameField>`

These two annotations work together with `smith.a.c/CrReadyWhenExistsKind` and `smith.a.c/CrReadyWhenExistsVersion`.
Please refer to their definition for definitions of `Tinst` and `K`.

`smith.a.c/CrOutputNameModeField` can be applied to a CRD `T` to configure the way name of the output object `O`
is constructed. Object `O` is a [Secret](https://kubernetes.io/docs/user-guide/secrets/) or a
[ConfigMap](https://kubernetes.io/docs/user-guide/configmap/) which holds parameters that may be injected into
object(s) `D` depending on `Tinst`. Object `O` is created/updated before `K` is created/updated.

The following values of `smith.a.c/CrOutputNameModeField` are defined:
- `Random` - `O`'s name is randomly generated on each `O` update.
- `ContentsHash` - `O`'s name is a deterministic hash function of `O`'s contents. `O`'s name changes if, and only if, it's contents change.
- `Fixed` - `O`'s name is provided via a field `<OutputNameSetterField>` on `Tinst`. Name of that field is defined by
`smith.a.c/CrOutputNameSetterField` set on `T`.

Each time `O`'s name changes a new `O` object is created, all dependent objects will have references updated and the old object will be deleted.

Smith does not use `smith.a.c/CrOutputNameModeField` and `smith.a.c/CrOutputNameSetterField` annotations directly,
they are defined here for completeness only. It is the job of a CR controller to honor/support them and it is up to
whoever constructs the Bundle object with that CR to use those fields or not.

`smith.a.c/CrOutputNameField` is the name of a field on `K` that contains the actual name of `O` that has been created.

Example of a CRD `T`:

```yaml
apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  name: resource-claims.smith.atlassian.com
  annotations:
    smith.atlassian.com/CrReadyWhenExistsKind: ResourceBinding
    smith.atlassian.com/CrReadyWhenExistsVersion: smith.atlassian.com/v1
    smith.atlassian.com/CrOutputNameModeField: spec.secretNameMode
    smith.atlassian.com/CrOutputNameSetterField: spec.secretName
    smith.atlassian.com/CrOutputNameField: status.secretName
spec:
  group: smith.atlassian.com
  version: v1
  names:
    kind: ResourceClaim
    plural: resourceclaims
    singular: resourceclaim
```

Example of a CR `Tinst`:

```yaml
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

```yaml
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

```yaml
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
