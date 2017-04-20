// +build integration

package integration_tests

import (
	"testing"

	"github.com/atlassian/smith"

	"github.com/atlassian/smith/examples/tprattribute"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	apiv1 "k8s.io/client-go/pkg/api/v1"
)

const (
	useNamespace = apiv1.NamespaceDefault
)

func assertCondition(t *testing.T, bundle *smith.Bundle, conditionType smith.BundleConditionType, status apiv1.ConditionStatus) {
	_, condition := bundle.GetCondition(conditionType)
	if assert.NotNil(t, condition) {
		assert.Equal(t, status, condition.Status)
	}
}

func smithScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	scheme.AddUnversionedTypes(apiv1.SchemeGroupVersion, &metav1.Status{})
	smith.AddToScheme(scheme)
	return scheme
}

func sleeperScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	scheme.AddUnversionedTypes(apiv1.SchemeGroupVersion, &metav1.Status{})
	tprattribute.AddToScheme(scheme)
	return scheme
}
