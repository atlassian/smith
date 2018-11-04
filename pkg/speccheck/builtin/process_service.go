package builtin

import (
	"github.com/atlassian/smith/pkg/speccheck"
	"github.com/atlassian/smith/pkg/util"
	core_v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

type service struct {
}

func (service) ApplySpec(ctx *speccheck.Context, spec, actual *unstructured.Unstructured) (runtime.Object, error) {
	var serviceSpec core_v1.Service
	if err := util.ConvertType(coreV1Scheme, spec, &serviceSpec); err != nil {
		return nil, err
	}
	var serviceActual core_v1.Service
	if err := util.ConvertType(coreV1Scheme, actual, &serviceActual); err != nil {
		return nil, err
	}

	serviceSpec.Spec.ClusterIP = serviceActual.Spec.ClusterIP
	serviceSpec.Status = serviceActual.Status

	if len(serviceActual.Spec.Ports) == len(serviceSpec.Spec.Ports) {
		for i, port := range serviceSpec.Spec.Ports {
			if port.NodePort == 0 {
				actualPort := serviceActual.Spec.Ports[i]
				port.NodePort = actualPort.NodePort
				if port == actualPort { // NodePort field is the only difference, other fields are the same
					// Copy port from actual if port is not specified in spec
					serviceSpec.Spec.Ports[i].NodePort = actualPort.NodePort
				}
			}
		}
	}

	return &serviceSpec, nil
}
