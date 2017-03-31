package app

import (
	"github.com/atlassian/smith"

	"k8s.io/apimachinery/pkg/runtime/schema"
	extensions "k8s.io/client-go/pkg/apis/extensions/v1beta1"
)

var tprGVK = extensions.SchemeGroupVersion.WithKind("ThirdPartyResource")

var bundleGVK = schema.GroupVersionKind{
	Group:   smith.SmithResourceGroup,
	Version: smith.BundleResourceVersion,
	Kind:    smith.BundleResourceKind,
}

type Processor interface {
	Rebuild(*smith.Bundle)
}
