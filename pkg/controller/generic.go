package controller

import (
	"context"

	"github.com/ash2k/stager/wait"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type Generic struct {
	controllers map[schema.GroupVersionKind]Interface
}

func NewGeneric(config *Config, constructors ...Constructor) (*Generic, error) {
	controllers := make(map[schema.GroupVersionKind]Interface, len(constructors))
	for _, constr := range constructors {
		descr := constr.Describe()
		if _, ok := controllers[descr.GVK]; ok {
			return nil, errors.Errorf("duplicate controller for GVK %s", descr.GVK)
		}
		iface, err := constr.New(config)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to construct controller for GVK %s", descr.GVK)
		}
		controllers[descr.GVK] = iface
	}
	config.Controllers = controllers
	return &Generic{
		controllers: controllers,
	}, nil
}

func (g *Generic) Run(ctx context.Context) {
	var wg wait.Group
	defer wg.Wait()
	for _, c := range g.controllers {
		wg.StartWithContext(ctx, c.Run)
	}
	<-ctx.Done()
}
