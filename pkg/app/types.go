package app

import (
	"context"

	"github.com/atlassian/smith"
)

type Processor interface {
	Rebuild(context.Context, *smith.Bundle) error
}
