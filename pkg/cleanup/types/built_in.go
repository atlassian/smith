package types

import (
	"reflect"
	"strconv"
	"strings"

	"github.com/atlassian/smith"
	"github.com/atlassian/smith/pkg/cleanup"
	"github.com/atlassian/smith/pkg/util"
	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	apps_v1 "k8s.io/api/apps/v1"
	core_v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	MainKnownTypes = map[schema.GroupKind]cleanup.SpecCleanup{
		{Group: apps_v1.GroupName, Kind: "Deployment"}: deploymentCleanup,
		{Group: core_v1.GroupName, Kind: "Service"}:    serviceCleanup,
		{Group: core_v1.GroupName, Kind: "Secret"}:     secretCleanup,
	}

	ServiceCatalogKnownTypes = map[schema.GroupKind]cleanup.SpecCleanup{
		{Group: sc_v1b1.GroupName, Kind: "ServiceBinding"}:  bindingCleanup,
		{Group: sc_v1b1.GroupName, Kind: "ServiceInstance"}: instanceCleanup,
	}

	appsV1Scheme = runtime.NewScheme()
	scV1B1Scheme = runtime.NewScheme()
	coreV1Scheme = runtime.NewScheme()
)

// LastAppliedReplicasAnnotation is the name of annotation which stores last applied replicas for deployment
const LastAppliedReplicasAnnotation string = smith.Domain + "/LastAppliedReplicas"

func init() {
	err := apps_v1.SchemeBuilder.AddToScheme(appsV1Scheme)
	if err != nil {
		panic(err)
	}
	err = sc_v1b1.SchemeBuilder.AddToScheme(scV1B1Scheme)
	if err != nil {
		panic(err)
	}
	err = core_v1.SchemeBuilder.AddToScheme(coreV1Scheme)
	if err != nil {
		panic(err)
	}
}

func deploymentCleanup(cleanupCtx *cleanup.Context, spec, actual *unstructured.Unstructured) (runtime.Object, error) {
	var deploymentSpec apps_v1.Deployment
	if err := util.ConvertType(appsV1Scheme, spec, &deploymentSpec); err != nil {
		return nil, err
	}
	var deploymentActual apps_v1.Deployment
	if err := util.ConvertType(appsV1Scheme, actual, &deploymentActual); err != nil {
		return nil, err
	}

	deploymentSpec.Spec.Template.Spec.DeprecatedServiceAccount = deploymentSpec.Spec.Template.Spec.ServiceAccountName

	/*
		update replicas based on LastAppliedReplicas annotation and running config
		to avoid conflicts with other controllers like HPA
	*/

	if deploymentSpec.Spec.Replicas == nil {
		return util.RuntimeToUnstructured(&deploymentSpec)
	}

	if deploymentSpec.Annotations == nil {
		deploymentSpec.Annotations = make(map[string]string)
	}

	lastAppliedReplicasConf, ok := deploymentActual.Annotations[LastAppliedReplicasAnnotation]
	if !ok {
		// add LastAppliedReplicas annotation if it doesn't exist
		deploymentSpec.Annotations[LastAppliedReplicasAnnotation] = strconv.Itoa(int(*deploymentSpec.Spec.Replicas))
		return util.RuntimeToUnstructured(&deploymentSpec)
	}

	// Parse last applied replicas from running config's annotation
	// overrides with current replicas inside spec if parsing failure
	lastAppliedReplicas, err := strconv.Atoi(strings.TrimSpace(lastAppliedReplicasConf))
	if err != nil {
		cleanupCtx.Logger.Warn("overriding last applied replicas annotation due to parsing failure", zap.Error(err))
		deploymentSpec.Annotations[LastAppliedReplicasAnnotation] = strconv.Itoa(int(*deploymentSpec.Spec.Replicas))
		return util.RuntimeToUnstructured(&deploymentSpec)
	}

	// spec changed => update annotations and use spec replicas config
	if *deploymentSpec.Spec.Replicas != int32(lastAppliedReplicas) {
		deploymentSpec.Annotations[LastAppliedReplicasAnnotation] = strconv.Itoa(int(*deploymentSpec.Spec.Replicas))
	} else {
		// spec not changed => use actual running config if it exists
		// since it might be updated by other controller like HPA
		// otherwise use spec replicas config
		if deploymentActual.Spec.Replicas != nil {
			*deploymentSpec.Spec.Replicas = *deploymentActual.Spec.Replicas
		}
	}

	return util.RuntimeToUnstructured(&deploymentSpec)
}

func serviceCleanup(cleanupCtx *cleanup.Context, spec, actual *unstructured.Unstructured) (runtime.Object, error) {
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

func secretCleanup(cleanupCtx *cleanup.Context, spec, actual *unstructured.Unstructured) (runtime.Object, error) {
	var secretSpec core_v1.Secret
	if err := util.ConvertType(coreV1Scheme, spec, &secretSpec); err != nil {
		return nil, err
	}

	// StringData overwrites Data
	if len(secretSpec.StringData) > 0 {
		if secretSpec.Data == nil {
			secretSpec.Data = make(map[string][]byte, len(secretSpec.StringData))
		}
		for k, v := range secretSpec.StringData {
			secretSpec.Data[k] = []byte(v)
		}
		secretSpec.StringData = nil
	}

	return &secretSpec, nil
}

func bindingCleanup(cleanupCtx *cleanup.Context, spec, actual *unstructured.Unstructured) (runtime.Object, error) {
	var sbSpec sc_v1b1.ServiceBinding
	if err := util.ConvertType(scV1B1Scheme, spec, &sbSpec); err != nil {
		return nil, err
	}
	var sbActual sc_v1b1.ServiceBinding
	if err := util.ConvertType(scV1B1Scheme, actual, &sbActual); err != nil {
		return nil, err
	}

	// managed by service catalog auth filtering just copy to make the comparison work
	sbSpec.Spec.UserInfo = sbActual.Spec.UserInfo

	err := setEmptyFieldsFromActual(&sbSpec.Spec, &sbActual.Spec, []string{
		// users should never set these ref fields
		"ServiceInstanceRef",

		// users may set these fields, generally they are autogenerated
		"ExternalID",
	})
	if err != nil {
		return nil, err
	}

	return &sbSpec, nil
}

func instanceCleanup(cleanupCtx *cleanup.Context, spec, actual *unstructured.Unstructured) (runtime.Object, error) {
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

	err := setEmptyFieldsFromActual(&instanceSpec.Spec, &instanceActual.Spec, []string{
		// users should never set these ref fields
		"ClusterServiceClassRef",
		"ClusterServicePlanRef",
		"ServiceClassRef",
		"ServicePlanRef",

		// users may set these fields, generally they are autogenerated
		"UpdateRequests",
		"ExternalID",
	})
	if err != nil {
		return nil, err
	}

	return &instanceSpec, nil
}

// setFieldsFromActual mutates the target with fields from the instantiated object,
// iff that field was not set in the original object.
func setEmptyFieldsFromActual(requested, actual interface{}, fields []string) error {
	requestedValue := reflect.ValueOf(requested).Elem()
	actualValue := reflect.ValueOf(actual).Elem()

	if requestedValue.Type() != actualValue.Type() {
		return errors.Errorf("attempted to set fields from different types: %q from %q",
			requestedValue, actualValue)
	}

	for _, field := range fields {
		requestedField := requestedValue.FieldByName(field)
		if !requestedField.IsValid() {
			return errors.Errorf("no such field %q to cleanup", field)
		}
		actualField := actualValue.FieldByName(field)
		if !actualField.IsValid() {
			return errors.Errorf("no such field %q to cleanup", field)
		}

		if reflect.DeepEqual(requestedField.Interface(), reflect.Zero(requestedField.Type()).Interface()) {
			requestedField.Set(actualField)
		}
	}

	return nil
}
