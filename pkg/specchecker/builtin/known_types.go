package builtin

import (
	"github.com/atlassian/smith/pkg/specchecker"
	sc_v1b1 "github.com/kubernetes-sigs/service-catalog/pkg/apis/servicecatalog/v1beta1"
	apps_v1 "k8s.io/api/apps/v1"
	core_v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	MainKnownTypes = map[schema.GroupKind]specchecker.ObjectProcessor{
		{Group: apps_v1.GroupName, Kind: "Deployment"}: deployment{},
		{Group: core_v1.GroupName, Kind: "Service"}:    service{},
		{Group: core_v1.GroupName, Kind: "Secret"}:     secret{},
	}

	ServiceCatalogKnownTypes = map[schema.GroupKind]specchecker.ObjectProcessor{
		{Group: sc_v1b1.GroupName, Kind: "ServiceBinding"}:  serviceBinding{},
		{Group: sc_v1b1.GroupName, Kind: "ServiceInstance"}: serviceInstance{},
	}
)
