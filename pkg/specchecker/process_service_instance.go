package specchecker

import (
	"github.com/atlassian/smith/pkg/util"
	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

type serviceInstance struct {
}

func (serviceInstance) ApplySpec(ctx *Context, spec, actual *unstructured.Unstructured) (runtime.Object, error) {
	var instanceSpec sc_v1b1.ServiceInstance
	if err := util.ConvertType(scV1B1Scheme, spec, &instanceSpec); err != nil {
		return nil, err
	}
	var instanceActual sc_v1b1.ServiceInstance
	if err := util.ConvertType(scV1B1Scheme, actual, &instanceActual); err != nil {
		return nil, err
	}

	instanceSpec.ObjectMeta.Finalizers = instanceActual.ObjectMeta.Finalizers
	// managed by service catalog auth filtering just copy to make the comparison work
	instanceSpec.Spec.UserInfo = instanceActual.Spec.UserInfo

	err := setEmptyFieldsFromActual(&instanceSpec.Spec, &instanceActual.Spec,
		// users should never set these ref fields
		"ClusterServiceClassRef",
		"ClusterServicePlanRef",
		"ServiceClassRef",
		"ServicePlanRef",

		// users may set these fields, generally they are autogenerated
		"UpdateRequests",
		"ExternalID",
	)
	if err != nil {
		return nil, err
	}

	return &instanceSpec, nil
}
