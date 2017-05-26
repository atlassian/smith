package app

import (
	"context"

	"github.com/atlassian/smith"

	ext_v1b1 "k8s.io/client-go/pkg/apis/extensions/v1beta1"
)

var (
	tprGVK = ext_v1b1.SchemeGroupVersion.WithKind("ThirdPartyResource")
)

type Processor interface {
	Rebuild(context.Context, *smith.Bundle) error
}
