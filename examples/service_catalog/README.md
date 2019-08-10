# Service Catalog example

Recoding of the presentation to Service Catalog SIG: https://youtu.be/7fgPgtQh5Es

## Description

Imagine that we want to deploy a service to Kubernetes, which consists of:
- Running application instances
- Postgres database, SQS queue
- Load balancer

![Application diagram](img/Application_example.png?raw=true "Application diagram")

For provisioning resources in AWS we can use [Service Catalog](https://github.com/kubernetes-sigs/service-catalog).
It allows us to provision resources through [Open Service Broker API](https://github.com/openservicebrokerapi/servicebroker)
and produce Kubernetes Secret objects with credentials which we can inject as environment variables into Pods using PodPresets.

The resulting graph of Kubernetes and Service Catalog objects consists of:
- `Pod`s for running application instances
- `PodPreset`s referencing `Secret`s and injecting them into `Pod`s
- Service Catalog `ServiceInstance` and `ServiceBinding` objects
- `Ingress` and `Service` objects for load balancing

![Kubernetes objects graph](img/Kubernetes_graph.png?raw=true "Kubernetes objects graph")

## Problem

The objects in this graph should be created in particular order, in respect of dependencies between them (note: independent branches in the graph could be processed in parallel).
There is no built-in generic way in Kubernetes of making sure that the objects are created in a particular order. And **Smith** project was designed to solve exactly this problem.

## Demo

Smith Bundle declaring the graph described above (see [application.yaml](application.yaml)):

```yaml
apiVersion: smith.atlassian.com/v1
kind: Bundle
metadata:
  name: sampleapp
spec:
  resources:

  - name: instance1
    spec:
      apiVersion: servicecatalog.k8s.io/v1beta1
      kind: ServiceInstance
      metadata:
        name: instance1
      spec:
        externalID: 786abffa-ed42-4c70-a06f-3836fe3d641c
        serviceClassName: user-provided-service
        planName: default
        parameters:
          credentials:
            token: token

  - name: binding1
    references:
    - name: instance1-metadata-name
      resource: instance1
      path: metadata.name
    spec:
      apiVersion: servicecatalog.k8s.io/v1beta1
      kind: ServiceBinding
      metadata:
        name: binding1
      spec:
        instanceRef:
          name: "!{instance1-metadata-name}"
        secretName: secret1

  - name: binding2
    references:
    - name: instance1-metadata-name
      resource: instance1
      path: metadata.name
    spec:
      apiVersion: servicecatalog.k8s.io/v1beta1
      kind: ServiceBinding
      metadata:
        name: binding2
      spec:
        instanceRef:
          name: "!{instance1-metadata-name}"
        secretName: secret2

  - name: podpreset1
    references:
    - name: binding1-secretName
      resource: binding1
      path: spec.secretName
    - name: binding2-secretName
      resource: binding2
      path: spec.secretName
    spec:
      apiVersion: settings.k8s.io/v1alpha1
      kind: PodPreset
      metadata:
        name: podpreset1
      spec:
        selector:
          matchLabels:
            role: app
        envFrom:
        - prefix: BINDING1_
          secretRef:
            name: "!{binding1-secretName}"
        - prefix: BINDING2_
          secretRef:
            name: "!{binding2-secretName}"

  - name: deployment1
    references:
    - name: podpreset1-matchLabels
      resource: podpreset1
      path: spec.selector.matchLabels
    spec:
      apiVersion: apps/v1beta2
      kind: Deployment
      metadata:
        name: deployment1
      spec:
        replicas: 2
        template:
          metadata:
            labels: "!{podpreset1-matchLabels}"
          spec:
            containers:
            - name: nginx
              image: nginx:latest
              ports:
              - containerPort: 80

  - name: service1
    references:
    - name: deployment1-labels
      resource: deployment1
      path: spec.template.metadata.labels
    spec:
      apiVersion: v1
      kind: Service
      metadata:
        name: service1
      spec:
        ports:
        - port: 80
          protocol: TCP
          targetPort: 80
          nodePort: 30090
        selector: "!{deployment1-labels}"
        type: NodePort

  - name: ingress1
    references:
    - name: service1-metadata-name
      resource: service1
      path: metadata.name
    - name: service1-port
      resource: service1
      path: spec.ports[?(@.protocol=="TCP")].port
    spec:
      apiVersion: extensions/v1beta1
      kind: Ingress
      metadata:
        name: ingress1
      spec:
        rules:
        - http:
            paths:
            - path: /
              backend:
                serviceName: "!{service1-metadata-name}"
                servicePort: "!{service1-port}"
```

### Screencast

[![asciicast](img/asciinema.png)](https://asciinema.org/a/125263)
