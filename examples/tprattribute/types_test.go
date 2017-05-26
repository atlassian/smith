package tprattribute

import (
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var _ runtime.Object = &SleeperList{}
var _ meta_v1.ListMetaAccessor = &SleeperList{}

var _ runtime.Object = &Sleeper{}
var _ meta_v1.ObjectMetaAccessor = &Sleeper{}
