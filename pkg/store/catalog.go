// Package catalog handles interacting with the OSB catalog endpoint
// (i.e. informers/helpers for ClusterServiceClass and ClusterServicePlan)
package store

import (
	"fmt"
	"strings"

	"sync"

	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	"github.com/pkg/errors"
	"github.com/xeipuuv/gojsonschema"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
)

type planSchemaAction string
type planSchemaKey string

const (
	serviceClassExternalNameIndex        = "ServiceClassExternalNameIndex"
	serviceClassAndPlanExternalNameIndex = "ServiceClassAndPlanExternalNameIndex"
	instanceCreateAction                 = planSchemaAction("instanceCreate")
	instanceUpdateAction                 = planSchemaAction("instanceUpdate")
	bindingCreateAction                  = planSchemaAction("bindingCreate")
)

type schemaWithResourceVersion struct {
	resourceVersion string
	schema          *gojsonschema.Schema
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
		schemas:                make(map[planSchemaKey]schemaWithResourceVersion),
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

func makePlanSchemaKey(plan *sc_v1b1.ClusterServicePlan, action planSchemaAction) planSchemaKey {
	return planSchemaKey(fmt.Sprintf("%s/%s/%s", plan.Name, action))
}

func (c *Catalog) getSchemaCache(plan *sc_v1b1.ClusterServicePlan, action planSchemaAction) (*gojsonschema.Schema, bool) {
	key := makePlanSchemaKey(plan, action)

	c.schemasRWMutex.RLock()
	defer c.schemasRWMutex.RUnlock()
	if schemaWithRv, ok := c.schemas[key]; ok && schemaWithRv.resourceVersion == plan.ResourceVersion {
		return schemaWithRv.schema, true
	} else {
		// nil is a valid entry in the cache
		return nil, false
	}
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
		rawSchema = plan.Spec.ServiceInstanceCreateParameterSchema
	case instanceUpdateAction:
		rawSchema = plan.Spec.ServiceInstanceUpdateParameterSchema
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

func (c *Catalog) ValidateServiceInstanceSpec(serviceInstanceSpec *sc_v1b1.ServiceInstanceSpec) error {
	if len(serviceInstanceSpec.ParametersFrom) > 0 {
		return errors.New("cannot validate ServiceInstanceSpec which has a ParametersFrom block (insufficient information)")
	}

	servicePlan, err := c.GetPlanOf(serviceInstanceSpec)
	if err != nil {
		return err
	}

	// We ignore the update schema here and assume it's equivalent to
	// create (since kubernetes/service catalog can't properly distinguish
	// them anyway as there are currently no true PATCH updates).
	schema, err := c.getParsedSchema(servicePlan, instanceCreateAction)
	if err != nil {
		return err
	}
	if schema == nil {
		// no schema to validate against anyway
		return nil
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
	validationResult, err := schema.Validate(gojsonschema.NewBytesLoader(parameters))
	if err != nil {
		return errors.Wrapf(err, "error validating osb resource parameters for %q", servicePlan.Spec.ExternalName)
	}

	if !validationResult.Valid() {
		validationErrors := validationResult.Errors()
		msgs := make([]string, 0, len(validationErrors))

		for _, validationErr := range validationErrors {
			msgs = append(msgs, validationErr.String())
		}

		return errors.Errorf("spec failed validation against schema: %s",
			strings.Join(msgs, ", "))
	}

	return nil
}
