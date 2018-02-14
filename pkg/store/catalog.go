// Package catalog handles interacting with the OSB catalog endpoint
// (i.e. informers/helpers for ClusterServiceClass and ClusterServicePlan)
package store

import (
	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	"github.com/pkg/errors"
	"k8s.io/client-go/tools/cache"
)

type planSchemaAction string
type planSchemaKey string
type planResourceVersionKey string

const (
	serviceClassExternalNameIndex        = "ServiceClassExternalNameIndex"
	serviceClassAndPlanExternalNameIndex = "ServiceClassAndPlanExternalNameIndex"
)

// Catalog is a convenience interface to access OSB catalog information
type Catalog struct {
	serviceClassInfIndexer cache.Indexer
	servicePlanInfIndexer  cache.Indexer
}

func NewCatalog(serviceClassInf cache.SharedIndexInformer, servicePlanInf cache.SharedIndexInformer) *Catalog {
	serviceClassInf.AddIndexers(cache.Indexers{
		serviceClassExternalNameIndex: func(obj interface{}) ([]string, error) {
			serviceClass := obj.(*sc_v1b1.ClusterServiceClass)
			return []string{serviceClass.Spec.ExternalName}, nil
		},
	})
	servicePlanInf.AddIndexers(cache.Indexers{
		serviceClassAndPlanExternalNameIndex: func(obj interface{}) ([]string, error) {
			servicePlan := obj.(*sc_v1b1.ClusterServicePlan)
			return []string{serviceClassAndPlanExternalNameIndexKey(servicePlan.Spec.ClusterServiceClassRef.Name, servicePlan.Spec.ExternalName)}, nil
		},
	})

	return &Catalog{
		serviceClassInfIndexer: serviceClassInf.GetIndexer(),
		servicePlanInfIndexer:  servicePlanInf.GetIndexer(),
	}
}

func serviceClassAndPlanExternalNameIndexKey(serviceClassName string, servicePlanExternalName string) string {
	return serviceClassName + "/" + servicePlanExternalName
}

func (c *Catalog) GetClassOf(serviceInstanceSpec *sc_v1b1.ServiceInstanceSpec) (*sc_v1b1.ClusterServiceClass, error) {
	if serviceInstanceSpec.ClusterServiceClassName == "" && serviceInstanceSpec.ClusterServiceClassExternalName == "" {
		return nil, errors.Errorf("ServiceInstance must have at least ClusterServiceClassName or ClusterServiceExternalName")
	}
	if serviceInstanceSpec.ClusterServiceClassName != "" && serviceInstanceSpec.ClusterServiceClassExternalName != "" {
		// Not sure if this is true. Maybe ok if they match? But silly.
		return nil, errors.Errorf("ServiceInstance must have only one of ClusterServiceClassName or ClusterServiceExternalName")
	}

	if serviceInstanceSpec.ClusterServiceClassName != "" {
		item, exists, err := c.serviceClassInfIndexer.GetByKey(serviceInstanceSpec.ClusterServiceClassName)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		if !exists {
			return nil, errors.Errorf("ServiceInstance refers to non-existant ClusterServiceClass %q", serviceInstanceSpec.ClusterServiceClassName)
		}
		return item.(*sc_v1b1.ClusterServiceClass), nil
	}

	items, err := c.serviceClassInfIndexer.ByIndex(serviceClassExternalNameIndex, serviceInstanceSpec.ClusterServiceClassExternalName)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	switch len(items) {
	case 0:
		return nil, errors.Errorf("ServiceInstance refers to non-existant ClusterServiceClass %q", serviceInstanceSpec.ClusterServiceClassExternalName)
	case 1:
		return items[0].(*sc_v1b1.ClusterServiceClass), nil
	default:
		return nil, errors.Errorf("informer reported multiple instances for ClusterServiceClass %q", serviceInstanceSpec.ClusterServiceClassExternalName)
	}

}

func (c *Catalog) GetPlanOf(serviceInstanceSpec *sc_v1b1.ServiceInstanceSpec) (*sc_v1b1.ClusterServicePlan, error) {
	if serviceInstanceSpec.ClusterServicePlanName == "" && serviceInstanceSpec.ClusterServicePlanExternalName == "" {
		// TODO actually, this is ok according to Service Catalog if there is only one plan.
		// This is annoying to do well, though, because I think we have to setup another index.
		return nil, errors.Errorf("ServiceInstance must have at least ClusterServicePlanName or ClusterServiceExternalName")
	}
	if serviceInstanceSpec.ClusterServicePlanName != "" && serviceInstanceSpec.ClusterServicePlanExternalName != "" {
		// Not sure if this is true. Maybe ok if they match? But silly.
		return nil, errors.Errorf("ServiceInstance must have only one of ClusterServicePlanName or ClusterServiceExternalName")
	}

	if serviceInstanceSpec.ClusterServicePlanName != "" {
		item, exists, err := c.servicePlanInfIndexer.GetByKey(serviceInstanceSpec.ClusterServicePlanName)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		if !exists {
			return nil, errors.Errorf("ServiceInstance refers to non-existant plan %q", serviceInstanceSpec.ClusterServicePlanName)
		}
		return item.(*sc_v1b1.ClusterServicePlan), nil
	}

	// If we don't have the plan UUID, we need to look up the class to find its UUID
	serviceClass, err := c.GetClassOf(serviceInstanceSpec)
	if err != nil {
		return nil, err
	}

	planKey := serviceClassAndPlanExternalNameIndexKey(serviceClass.Name, serviceInstanceSpec.ClusterServicePlanExternalName)
	items, err := c.servicePlanInfIndexer.ByIndex(serviceClassAndPlanExternalNameIndex, planKey)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	switch len(items) {
	case 0:
		return nil, errors.Errorf("ServiceInstance refers to non-existant ClusterServicePlan %q", planKey)
	case 1:
		return items[0].(*sc_v1b1.ClusterServicePlan), nil
	default:
		return nil, errors.Errorf("informer reported multiple instances for ClusterServicePlan %q", planKey)
	}
}
