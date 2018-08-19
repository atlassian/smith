# Field references

Cross-object field references are a way for an object to get output field(s) of its dependency
(or multiple dependencies) injected into it as a field (or multiple fields). References to only direct
dependencies are supported.

References are defined as part of resource definition. Each reference should have a `resource` field that specifies
the name of the referenced resource. A reference may also have:
- a `name` that can be used as a substitution marker when wrapped in `!{reference-name-here}`. The existing type
  is maintained. If `name` is not specified or is not used, the reference effectively becomes just an ordering
  constraint
- a `path` that can specify a JSON path expression to extract part(s) of the referenced resource
- an `example` that can specify an example of the value that is extracted using that reference. It is used for schema
  validation - see below for the detailed description
- a `modifier` that can specify an additional bit of information for the reference processor. Currently the only
  allowed value is `bindsecret` - see below for the detailed description

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
    references:
    - name: sleeper1-status-message
      resource: sleeper1
      path: status.message
    spec:
      object:
        apiVersion: crd.atlassian.com/v1
        kind: Sleeper
        metadata:
          name: sleeper2
        spec:
          sleepFor: 4
          wakeupMessage: "!{sleeper1-status-message}"
  - name: sleeper3
    references:
    - name: sleeper2-spec
      resource: sleeper2
      path: spec
    spec:
      object:
        apiVersion: crd.atlassian.com/v1
        kind: Sleeper
        metadata:
          name: sleeper3
        spec: "!{sleeper2-spec}"
```

## Referring to ServiceBinding outputs

When Service Catalog processes a `ServiceBinding`, the output is placed in a `Secret`
(since they might be secret). If they're not secret, it's convenient to directly
reference them in the bundle. This can be done by specifying `bindsecret` `modifier` attribute and then `path` as
if referring to a `Secret` resource. E.g. `path` set to `data.secretkey` will fetch the value stored with `secretkey`
key. Secrets inside data fields are stored base64 encoded in kubernetes, but when you refer to
them in Smith they are plain. At the moment, `bindsecret` is the only parameterisation of
the reference that is allowed.

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
              host: http://google.com

  - name: a-binding
    references:
    - name: a-metadata-name
      resource: a
      path: metadata.name
    spec:
      object:
        metadata:
          name: a-binding
        apiVersion: servicecatalog.k8s.io/v1beta1
        kind: ServiceBinding
        spec:
          instanceRef:
            name: "!{a-metadata-name}"
          secretName: a-binding-secret

  - name: b
    references:
    - name: a-binding-host
      resource: a-binding
      path: data.host
      modifier: bindsecret
    - name: a-binding-password
      resource: a-binding
      path: data.password
      modifier: bindsecret
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
            important: true
            host: "!{a-binding-host}"
            password: "!{a-binding-password}"
```

**Warning: it is not currently safe to have a truly secret field
passed this way (cf `a-binding-password`) unless it's inserted
into a `Secret`, as it will be exposed in the body of the created object.**
To do the example above correctly, we could instead construct a new `Secret` object
in the appropriate form for a `ServiceInstance` `parametersFrom` secret reference.
In future, this [should be automatic](https://github.com/atlassian/smith/issues/233).

## Early validation

Having references inside an object or plugin means that the final
shape of the object will only be known once the dependencies have
been evaluated. However, because we would like to fail as quickly
as possible if the user has entered invalid parameters, 'example'
values can be specified (as placeholders) so that `ServiceInstance`
objects and plugins can be evaluated against their JSON schemas.

Modifying part of the example from the previous section:

```yaml
  - name: b
    references:
    - name: a-binding-host
      resource: a-binding
      path: data.host
      example: http://example.com
      modifier: bindsecret
    - name: a-binding-password
      resource: a-binding
      path: data.password
      example: fakepassword
      modifier: bindsecret
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
            important: true
            host: "!{a-binding-host}"
            password: "!{a-binding-password}"
```

The difference here is that the `a-binding-host` and `a-binding-password` references now have an additional
`example` attribute with a sample value. This allows us
to validate the parameters against the JSON schema exposed
by the OSB catalog endpoint (and currently accessible via a field on
Service Catalog's `ClusterServicePlan` objects). Therefore the above
resource has the provisional `parameters` block for validation purposes
of:

```json
{
  "important": true,
  "host": "http://example.com",
  "password": "fakepassword",
}
```

This means we can check that `important: true` is reasonable and that we are
providing all required fields, though of course host/password themselves may
change. However, if references are used and examples are not provided,
this validation step is ignored.
