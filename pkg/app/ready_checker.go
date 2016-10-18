package app

import (
	"errors"

	"github.com/atlassian/smith"
)

var UnknownResourceKind = errors.New("unknown resource kind")

type StatusReadyChecker struct {
}

func (rc *StatusReadyChecker) IsReady(res *smith.Resource) (bool, error) {
	switch res.Spec.Kind {
	case "ConfigMap":
		return true, nil
	default:
		return false, UnknownResourceKind
	}
}
