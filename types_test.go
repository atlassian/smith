package smith

import (
	"k8s.io/client-go/pkg/api/meta"
	metav1 "k8s.io/client-go/pkg/apis/meta/v1"
	"k8s.io/client-go/pkg/runtime"
)

var _ runtime.Object = &TemplateList{}
var _ metav1.ListMetaAccessor = &TemplateList{}

var _ runtime.Object = &Template{}
var _ meta.ObjectMetaAccessor = &Template{}
