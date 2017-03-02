package tprattribute

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var _ runtime.Object = &SleeperList{}
var _ metav1.ListMetaAccessor = &SleeperList{}

var _ runtime.Object = &Sleeper{}
var _ metav1.ObjectMetaAccessor = &Sleeper{}
