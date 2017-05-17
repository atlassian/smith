package app

import (
	"context"

	"github.com/atlassian/smith"
	extensions "k8s.io/client-go/pkg/apis/extensions/v1beta1"
)

var (
	tprGVK = extensions.SchemeGroupVersion.WithKind("ThirdPartyResource")
)

type Processor interface {
	Rebuild(context.Context, *smith.Bundle) error
}
