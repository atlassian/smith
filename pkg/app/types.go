package app

import (
	"github.com/atlassian/smith"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

var tprGVK schema.GroupVersionKind = schema.GroupVersionKind{
	Group:   "extensions",
	Version: "v1beta1",
	Kind:    "ThirdPartyResource",
}

var bundleGVK schema.GroupVersionKind = schema.GroupVersionKind{
	Group:   smith.SmithResourceGroup,
	Version: smith.BundleResourceVersion,
	Kind:    smith.BundleResourceKind,
}

type Processor interface {
	Rebuild(*smith.Bundle)
}
