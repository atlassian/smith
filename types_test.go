package smith

import (
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var _ runtime.Object = &BundleList{}
var _ meta_v1.ListMetaAccessor = &BundleList{}

var _ runtime.Object = &Bundle{}
var _ meta_v1.ObjectMetaAccessor = &Bundle{}
