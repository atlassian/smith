package specchecker

import (
	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	"go.uber.org/zap"
	apps_v1 "k8s.io/api/apps/v1"
	core_v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	MainKnownTypes = map[schema.GroupKind]ObjectProcessor{
		{Group: apps_v1.GroupName, Kind: "Deployment"}: deployment{},
		{Group: core_v1.GroupName, Kind: "Service"}:    service{},
		{Group: core_v1.GroupName, Kind: "Secret"}:     secret{},
	}

	ServiceCatalogKnownTypes = map[schema.GroupKind]ObjectProcessor{
		{Group: sc_v1b1.GroupName, Kind: "ServiceBinding"}:  serviceBinding{},
		{Group: sc_v1b1.GroupName, Kind: "ServiceInstance"}: serviceInstance{},
	}
)

type ObjectProcessor interface {
	ApplySpec(ctx *Context, spec, actual *unstructured.Unstructured) (runtime.Object /*updatedSpec*/, error)
}

// Context includes objects used by different cleanup functions
type Context struct {
	Logger *zap.Logger
}
