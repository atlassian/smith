# Field references

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
      object:
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
      object:
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
      object:
        apiVersion: crd.atlassian.com/v1
        kind: Sleeper
        metadata:
          name: sleeper3
        spec: "{{{sleeper2#spec}}}"
```

## Referring to ServiceBinding outputs

When Service Catalog processes a ServiceBinding, the output is placed in a Secret
(since they might be secret). If they're not secret, it's convenient to directly
reference them in the bundle. This can be done by using `dependency:bindsecret#Data.secretkey`.
At the moment, `bindsecret` is the only parameterisation of the dependency that is allowed.

For example:

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
        metadata:
          name: ups-instance
        apiVersion: servicecatalog.k8s.io/v1beta1
        kind: ServiceInstance
        spec:
          clusterServiceClassExternalName: user-provided-service
          clusterServicePlanExternalName: default
          parameters:
            credentials:
              password: mypassword

  - name: a-binding
    dependsOn:
    - a
    spec:
      object:
        metadata:
          name: a-binding
        apiVersion: servicecatalog.k8s.io/v1beta1
        kind: ServiceBinding
        spec:
          instanceRef:
            name: "{{a#metadata.name}}"
          secretName: a-binding-secret

  - name: b
    dependsOn:
    - a-binding
    spec:
      object:
        metadata:
          name: ups-instance-2
        apiVersion: servicecatalog.k8s.io/v1beta1
        kind: ServiceInstance
        spec:
          clusterServiceClassExternalName: user-provided-service
          clusterServicePlanExternalName: default
          parameters:
            x: y
            password: "{{a-binding:bindsecret#password}}"
```
