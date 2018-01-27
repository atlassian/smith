#!/usr/bin/env bash

kubectl delete bundles/sampleapp
kubectl delete podpresets/podpreset1
kubectl delete deployments/deployment1
kubectl delete service/service1
kubectl delete ingress/ingress1
kubectl delete secrets/secret1
kubectl delete secrets/secret2
kubectl delete serviceinstance/instance1
kubectl delete servicebinding/binding1
kubectl delete servicebinding/binding2
