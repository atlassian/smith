#!/usr/bin/env bash

kubectl delete bundles/sampleapp
kubectl delete podpresets/podpreset1
kubectl delete deployments/deployment1
kubectl delete service/service1
kubectl delete ingress/ingress1
kubectl delete instances/instance1 --context=service-catalog
kubectl delete bindings/binding1 --context=service-catalog
kubectl delete bindings/binding2 --context=service-catalog
