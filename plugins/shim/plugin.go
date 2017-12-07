package main

import (
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	smith_plugin "github.com/atlassian/smith/pkg/plugin"
	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Process(resource smith_v1.Resource, dependencies map[smith_v1.ResourceName]smith_plugin.Dependency) (smith_plugin.ProcessResult, error) {
	// This is dumb, and just spits out a random service instance
	name := resource.PluginSpec.Name
	svcInstance := &sc_v1b1.ServiceInstance{
		ObjectMeta: meta_v1.ObjectMeta{
			Name: name,
		},
		TypeMeta: meta_v1.TypeMeta{
			APIVersion: sc_v1b1.SchemeGroupVersion.String(),
			Kind:       "ServiceInstance",
		},
		Spec: sc_v1b1.ServiceInstanceSpec{
			PlanReference: sc_v1b1.PlanReference{
				ClusterServiceClassName: "4f6e6cf6-ffdd-425f-a2c7-3c9258ad2468",
				ClusterServicePlanName:  "86064792-7ea2-467b-af93-ac9694d96d52",
			},
		},
	}

	return smith_plugin.ProcessResult{
		Object: svcInstance,
	}, nil
}

func IsSupported(plugin string) (bool, error) {
	return true, nil
}
