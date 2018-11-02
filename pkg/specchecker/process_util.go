package specchecker

import (
	"reflect"

	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	"github.com/pkg/errors"
	apps_v1 "k8s.io/api/apps/v1"
	core_v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
)

var (
	appsV1Scheme = runtime.NewScheme()
	scV1B1Scheme = runtime.NewScheme()
	coreV1Scheme = runtime.NewScheme()
)

func init() {
	utilruntime.Must(apps_v1.SchemeBuilder.AddToScheme(appsV1Scheme))
	utilruntime.Must(sc_v1b1.SchemeBuilder.AddToScheme(scV1B1Scheme))
	utilruntime.Must(core_v1.SchemeBuilder.AddToScheme(coreV1Scheme))
}

// setFieldsFromActual mutates the target with fields from the instantiated object,
// iff that field was not set in the original object.
func setEmptyFieldsFromActual(requested, actual interface{}, fields ...string) error {
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
