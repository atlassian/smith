// +build integration

package integration_tests

import (
	"testing"

	"github.com/atlassian/smith"

	"github.com/stretchr/testify/assert"
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
