package tprattribute

import (
	"k8s.io/client-go/pkg/api/meta"
	metav1 "k8s.io/client-go/pkg/apis/meta/v1"
	"k8s.io/client-go/pkg/runtime"
)

var _ runtime.Object = &SleeperList{}
var _ metav1.ListMetaAccessor = &SleeperList{}

var _ runtime.Object = &Sleeper{}
var _ meta.ObjectMetaAccessor = &Sleeper{}
