package smith

import (
	"k8s.io/client-go/pkg/api/meta"
	"k8s.io/client-go/pkg/runtime"
)

var _ runtime.Object = &TemplateList{}
var _ meta.ListMetaAccessor = &TemplateList{}

var _ runtime.Object = &Template{}
var _ meta.ObjectMetaAccessor = &Template{}
