package logz

import (
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func Resource(resourceName smith_v1.ResourceName) zapcore.Field {
	return zap.String("resource", string(resourceName))
}
