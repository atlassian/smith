package smith

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var _ runtime.Object = &TemplateList{}
var _ metav1.ListMetaAccessor = &TemplateList{}

var _ runtime.Object = &Template{}
var _ metav1.ObjectMetaAccessor = &Template{}
