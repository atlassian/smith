package app

import (
	"github.com/atlassian/smith"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var tprGVK metav1.GroupVersionKind = metav1.GroupVersionKind{
	Group:   "extensions",
	Version: "v1beta1",
	Kind:    "ThirdPartyResource",
}

var bundleGVK metav1.GroupVersionKind = metav1.GroupVersionKind{
	Group:   smith.SmithResourceGroup,
	Version: smith.BundleResourceVersion,
	Kind:    smith.BundleResourceKind,
}

type Processor interface {
	Rebuild(*smith.Bundle)
}
