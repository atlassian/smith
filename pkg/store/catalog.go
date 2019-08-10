// Package catalog handles interacting with the OSB catalog endpoint
// (i.e. informers/helpers for ClusterServiceClass and ClusterServicePlan)
package store

import (
	"fmt"
	"sync"

	sc_v1b1 "github.com/kubernetes-sigs/service-catalog/pkg/apis/servicecatalog/v1beta1"
	"github.com/pkg/errors"
	"github.com/xeipuuv/gojsonschema"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
)

type planSchemaAction string
type planSchemaKey string

const (
	serviceClassExternalNameIndex        = "ServiceClassExternalNameIndex"
	serviceClassExternalIDIndex          = "ServiceClassExternalIDIndex"
	serviceClassAndPlanExternalNameIndex = "ServiceClassAndPlanExternalNameIndex"
	servicePlanExternalIDIndex           = "ServicePlanExternalIDIndex"
	instanceCreateAction                 = planSchemaAction("instanceCreate")
	instanceUpdateAction                 = planSchemaAction("instanceUpdate")
	bindingCreateAction                  = planSchemaAction("bindingCreate")
)

type schemaWithResourceVersion struct {
	resourceVersion string
	schema          *gojsonschema.Schema
}

type ValidationResult struct {
	Errors []error
}

// Catalog is a convenience interface to access OSB catalog information
type Catalog struct {
	serviceClassInfIndexer cache.Indexer
	servicePlanInfIndexer  cache.Indexer

	// schemas is a cache of schemas by plan/action, but NOT resourceVersion.
	// However, we check ResourceVersion in the accessor to see if something is cached,
	// which means we don't hold old ResourceVersions around (but instead
	// replace them with an up-to-date version ASAP).
	// Unlike other parts of smith, this is an on-demand cache, and processing is
	// NOT currently triggered by addition/updates of plans.
	schemas        map[planSchemaKey]schemaWithResourceVersion
	schemasRWMutex sync.RWMutex
}

func NewCatalog(serviceClassInf cache.SharedIndexInformer, servicePlanInf cache.SharedIndexInformer) (*Catalog, error) {
	err := serviceClassInf.AddIndexers(cache.Indexers{
		serviceClassExternalNameIndex: func(obj interface{}) ([]string, error) {
			serviceClass := obj.(*sc_v1b1.ClusterServiceClass)
			return []string{serviceClass.Spec.ExternalName}, nil
		},
		serviceClassExternalIDIndex: func(obj interface{}) ([]string, error) {
			serviceClass := obj.(*sc_v1b1.ClusterServiceClass)
			return []string{serviceClass.Spec.ExternalID}, nil
		},
	})
	if err != nil {
		return nil, errors.WithStack(err)
	}
	err = servicePlanInf.AddIndexers(cache.Indexers{
		serviceClassAndPlanExternalNameIndex: func(obj interface{}) ([]string, error) {
			servicePlan := obj.(*sc_v1b1.ClusterServicePlan)
			return []string{serviceClassAndPlanExternalNameIndexKey(servicePlan.Spec.ClusterServiceClassRef.Name, servicePlan.Spec.ExternalName)}, nil
		},
		servicePlanExternalIDIndex: func(obj interface{}) ([]string, error) {
			servicePlan := obj.(*sc_v1b1.ClusterServicePlan)
			return []string{servicePlan.Spec.ExternalID}, nil
		},
	})
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return &Catalog{
		serviceClassInfIndexer: serviceClassInf.GetIndexer(),
		servicePlanInfIndexer:  servicePlanInf.GetIndexer(),
		schemas:                make(map[planSchemaKey]schemaWithResourceVersion),
	}, nil
}

func serviceClassAndPlanExternalNameIndexKey(serviceClassName string, servicePlanExternalName string) string {
	return serviceClassName + "/" + servicePlanExternalName
}

func (c *Catalog) GetClassOf(serviceInstanceSpec *sc_v1b1.ServiceInstanceSpec) (*sc_v1b1.ClusterServiceClass, error) {
	switch {
	case serviceInstanceSpec.ClusterServiceClassName != "" && serviceInstanceSpec.ClusterServiceClassExternalName != "":
		fallthrough
	case serviceInstanceSpec.ClusterServiceClassName != "" && serviceInstanceSpec.ClusterServiceClassExternalID != "":
		fallthrough
	case serviceInstanceSpec.ClusterServiceClassExternalName != "" && serviceInstanceSpec.ClusterServiceClassExternalID != "":
		return nil, errors.Errorf("ServiceInstance must have only one of ClusterServiceClassName or ClusterServiceClassExternalName or ClusterServiceClassExternalID")
	}
	switch {
	case serviceInstanceSpec.ClusterServiceClassName != "":
		item, exists, err := c.serviceClassInfIndexer.GetByKey(serviceInstanceSpec.ClusterServiceClassName)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		if !exists {
			return nil, errors.Errorf("ServiceInstance refers to non-existant ClusterServiceClass Name=%q", serviceInstanceSpec.ClusterServiceClassName)
		}
		return item.(*sc_v1b1.ClusterServiceClass), nil
	case serviceInstanceSpec.ClusterServiceClassExternalID != "":
		items, err := c.serviceClassInfIndexer.ByIndex(serviceClassExternalIDIndex, serviceInstanceSpec.ClusterServiceClassExternalID)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		switch len(items) {
		case 0:
			return nil, errors.Errorf("ServiceInstance refers to non-existant ClusterServiceClass ExternalID=%q", serviceInstanceSpec.ClusterServiceClassExternalID)
		case 1:
			return items[0].(*sc_v1b1.ClusterServiceClass), nil
		default:
			return nil, errors.Errorf("informer reported multiple instances for ClusterServiceClass ExternalID=%q", serviceInstanceSpec.ClusterServiceClassExternalID)
		}
	case serviceInstanceSpec.ClusterServiceClassExternalName != "":
		items, err := c.serviceClassInfIndexer.ByIndex(serviceClassExternalNameIndex, serviceInstanceSpec.ClusterServiceClassExternalName)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		switch len(items) {
		case 0:
			return nil, errors.Errorf("ServiceInstance refers to non-existent ClusterServiceClass ExternalName=%q", serviceInstanceSpec.ClusterServiceClassExternalName)
		case 1:
			return items[0].(*sc_v1b1.ClusterServiceClass), nil
		default:
			return nil, errors.Errorf("informer reported multiple instances for ClusterServiceClass ExternalName=%q", serviceInstanceSpec.ClusterServiceClassExternalName)
		}
	default:
		return nil, errors.Errorf("ServiceInstance must have at least ClusterServiceClassName or ClusterServiceExternalName or ClusterServiceClassExternalID")
	}
}

func (c *Catalog) GetPlanOf(serviceInstanceSpec *sc_v1b1.ServiceInstanceSpec) (*sc_v1b1.ClusterServicePlan, error) {
	switch {
	case serviceInstanceSpec.ClusterServicePlanName != "" && serviceInstanceSpec.ClusterServicePlanExternalName != "":
		fallthrough
	case serviceInstanceSpec.ClusterServicePlanName != "" && serviceInstanceSpec.ClusterServicePlanExternalID != "":
		fallthrough
	case serviceInstanceSpec.ClusterServicePlanExternalName != "" && serviceInstanceSpec.ClusterServicePlanExternalID != "":
		return nil, errors.Errorf("ServiceInstance must have only one of ClusterServicePlanName or ClusterServicePlanExternalName or ClusterServicePlanExternalID")
	}
	switch {
	case serviceInstanceSpec.ClusterServicePlanName != "":
		item, exists, err := c.servicePlanInfIndexer.GetByKey(serviceInstanceSpec.ClusterServicePlanName)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		if !exists {
			return nil, errors.Errorf("ServiceInstance refers to non-existent ClusterServicePlan Name=%q", serviceInstanceSpec.ClusterServicePlanName)
		}
		return item.(*sc_v1b1.ClusterServicePlan), nil
	case serviceInstanceSpec.ClusterServicePlanExternalID != "":
		items, err := c.servicePlanInfIndexer.ByIndex(servicePlanExternalIDIndex, serviceInstanceSpec.ClusterServicePlanExternalID)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		switch len(items) {
		case 0:
			return nil, errors.Errorf("ServiceInstance refers to non-existant ClusterServicePlan ExternalID=%q", serviceInstanceSpec.ClusterServicePlanExternalID)
		case 1:
			return items[0].(*sc_v1b1.ClusterServicePlan), nil
		default:
			return nil, errors.Errorf("informer reported multiple instances for ClusterServicePlan ExternalID=%q", serviceInstanceSpec.ClusterServicePlanExternalID)
		}
	case serviceInstanceSpec.ClusterServicePlanExternalName != "":
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
	default:
		return nil, errors.Errorf("ServiceInstance must have at least ClusterServiceClassName or ClusterServiceExternalName or ClusterServiceClassExternalID")
	}
}

func makePlanSchemaKey(plan *sc_v1b1.ClusterServicePlan, action planSchemaAction) planSchemaKey {
	return planSchemaKey(fmt.Sprintf("%s/%s", plan.Name, action))
}

func (c *Catalog) getSchemaCache(plan *sc_v1b1.ClusterServicePlan, action planSchemaAction) (*gojsonschema.Schema, bool) {
	key := makePlanSchemaKey(plan, action)

	c.schemasRWMutex.RLock()
	defer c.schemasRWMutex.RUnlock()
	if schemaWithRv, ok := c.schemas[key]; ok && schemaWithRv.resourceVersion == plan.ResourceVersion {
		return schemaWithRv.schema, true
	}
	// nil is a valid entry in the cache
	return nil, false
}

func (c *Catalog) setSchemaCache(plan *sc_v1b1.ClusterServicePlan, action planSchemaAction, schema *gojsonschema.Schema) {
	key := makePlanSchemaKey(plan, action)

	c.schemasRWMutex.Lock()
	defer c.schemasRWMutex.Unlock()
	c.schemas[key] = schemaWithResourceVersion{plan.ResourceVersion, schema}
}

func (c *Catalog) getParsedSchema(plan *sc_v1b1.ClusterServicePlan, action planSchemaAction) (*gojsonschema.Schema, error) {
	if schema, ok := c.getSchemaCache(plan, action); ok {
		return schema, nil
	}

	var rawSchema *runtime.RawExtension
	switch action {
	case instanceCreateAction:
		rawSchema = plan.Spec.InstanceCreateParameterSchema
	case instanceUpdateAction:
		rawSchema = plan.Spec.InstanceUpdateParameterSchema
	case bindingCreateAction:
		rawSchema = plan.Spec.ServiceBindingCreateParameterSchema
	default:
		return nil, errors.Errorf("plan action %q not understood", action)
	}

	var schema *gojsonschema.Schema
	if rawSchema == nil {
		schema = nil
	} else {
		var err error
		schema, err = gojsonschema.NewSchema(gojsonschema.NewBytesLoader(rawSchema.Raw))
		if err != nil {
			return nil, errors.Wrapf(err,
				"cannot parse json schema for plan %q on broker %q",
				plan.Spec.ExternalName, plan.Spec.ClusterServiceBrokerName)
		}
	}

	c.setSchemaCache(plan, action, schema)
	return schema, nil
}

func (c *Catalog) ValidateServiceInstanceSpec(serviceInstanceSpec *sc_v1b1.ServiceInstanceSpec) (ValidationResult, error) {
	if len(serviceInstanceSpec.ParametersFrom) > 0 {
		return ValidationResult{}, errors.New("cannot validate ServiceInstanceSpec which has a ParametersFrom block (insufficient information)")
	}

	servicePlan, err := c.GetPlanOf(serviceInstanceSpec)
	if err != nil {
		return ValidationResult{}, err
	}

	// We ignore the update schema here and assume it's equivalent to
	// create (since kubernetes/service catalog can't properly distinguish
	// them anyway as there are currently no true PATCH updates).
	schema, err := c.getParsedSchema(servicePlan, instanceCreateAction)
	if err != nil {
		return ValidationResult{
			Errors: []error{err},
		}, nil
	}
	if schema == nil {
		// no schema to validate against anyway
		return ValidationResult{}, nil
	}
	var parameters []byte
	if serviceInstanceSpec.Parameters != nil {
		parameters = serviceInstanceSpec.Parameters.Raw
	} else {
		// I'm not entirely sure what request ServiceCatalog ends up making when
		// no parameters at all are provided, but pretending it's an empty object
		// here makes testing more straight-forward and means that leaving out
		// parameters will give sane looking early validation failures...
		parameters = []byte("{}")
	}
	result, err := schema.Validate(gojsonschema.NewBytesLoader(parameters))
	if err != nil {
		return ValidationResult{}, errors.Wrapf(err, "error validating osb resource parameters for %q", servicePlan.Spec.ExternalName)
	}

	if !result.Valid() {
		validationErrors := result.Errors()
		errs := make([]error, 0, len(validationErrors))

		for _, validationErr := range validationErrors {
			errs = append(errs, errors.New(validationErr.String()))
		}

		return ValidationResult{errs}, nil
	}

	return ValidationResult{}, nil
}
