package logz

import (
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func Resource(resourceName smith_v1.ResourceName) zapcore.Field {
	return zap.String("resource", string(resourceName))
}

func Gvk(gvk schema.GroupVersionKind) zapcore.Field {
	return zap.Stringer("gvk", gvk)
}

func Object(obj meta_v1.Object) zapcore.Field {
	return ObjectName(obj.GetName())
}

func ObjectName(name string) zapcore.Field {
	return zap.String("object_name", name)
}

func Namespace(obj meta_v1.Object) zapcore.Field {
	return NamespaceName(obj.GetNamespace())
}

func NamespaceName(namespace string) zapcore.Field {
	if namespace == "" {
		return zap.Skip()
	}
	return zap.String("namespace", namespace)
}

func Bundle(bundle *smith_v1.Bundle) zapcore.Field {
	return BundleName(bundle.Name)
}

func BundleName(name string) zapcore.Field {
	return zap.String("bundle_name", name)
}
