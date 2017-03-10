package smith

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var _ runtime.Object = &BundleList{}
var _ metav1.ListMetaAccessor = &BundleList{}

var _ runtime.Object = &Bundle{}
var _ metav1.ObjectMetaAccessor = &Bundle{}
