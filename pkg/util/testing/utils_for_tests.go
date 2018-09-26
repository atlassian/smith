package testing

import (
	"testing"

	cond_v1 "github.com/atlassian/ctrl/apis/condition/v1"
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/stretchr/testify/assert"
)

func AssertCondition(t *testing.T, bundle *smith_v1.Bundle, conditionType cond_v1.ConditionType, status cond_v1.ConditionStatus) *cond_v1.Condition {
	_, condition := cond_v1.FindCondition(bundle.Status.Conditions, conditionType)
	if assert.NotNil(t, condition) {
		// TODO string casts are a workaround for https://github.com/stretchr/testify/issues/644
		assert.Equal(t, string(status), string(condition.Status), "%s: %s: %s", conditionType, condition.Reason, condition.Message)
	}
	return condition
}

func AssertResourceCondition(t *testing.T, bundle *smith_v1.Bundle, resName smith_v1.ResourceName, conditionType cond_v1.ConditionType, status cond_v1.ConditionStatus) *cond_v1.Condition {
	_, resStatus := bundle.Status.GetResourceStatus(resName)
	if !assert.NotNil(t, resStatus, "%s", resName) {
		return nil
	}
	_, condition := cond_v1.FindCondition(resStatus.Conditions, conditionType)
	if !assert.NotNil(t, condition, "%s: %s", resName, conditionType) {
		return nil
	}
	assert.Equal(t, status, condition.Status, "%s: %s: %s: %s", resName, conditionType, condition.Reason, condition.Message)
	return condition
}
