package testing

import (
	"testing"

	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"

	"github.com/stretchr/testify/assert"
)

func AssertCondition(t *testing.T, bundle *smith_v1.Bundle, conditionType smith_v1.BundleConditionType, status smith_v1.ConditionStatus) *smith_v1.BundleCondition {
	_, condition := bundle.GetCondition(conditionType)
	if assert.NotNil(t, condition) {
		assert.Equal(t, status, condition.Status, "%s: %s: %s", conditionType, condition.Reason, condition.Message)
	}
	return condition
}

func AssertResourceCondition(t *testing.T, bundle *smith_v1.Bundle, resName smith_v1.ResourceName, conditionType smith_v1.ResourceConditionType, status smith_v1.ConditionStatus) *smith_v1.ResourceCondition {
	_, resStatus := bundle.Status.GetResourceStatus(resName)
	if !assert.NotNil(t, resStatus, "%s", resName) {
		return nil
	}
	_, condition := resStatus.GetCondition(conditionType)
	if !assert.NotNil(t, condition, "%s: %s", resName, conditionType) {
		return nil
	}
	assert.Equal(t, status, condition.Status, "%s: %s: %s: %s", resName, conditionType, condition.Reason, condition.Message)
	return condition
}
