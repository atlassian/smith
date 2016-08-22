package app

import (
	"context"
	"fmt"
	"log"

	"github.com/ash2k/smith"
	"github.com/ash2k/smith/pkg/client"
)

type App struct {
	Version   string
	GitCommit string
}

func (a *App) Run(ctx context.Context) error {
	c, err := client.NewInCluster()
	if err != nil {
		return err
	}
	c.Agent = "smith/" + a.Version + "/" + a.GitCommit
	if err = ensureResourceExists(ctx, c); err != nil {
		return err
	}

	//c.List(ctx, ResourceGroupVersion, Namespace, )

	<-ctx.Done()
	return ctx.Err()
}

func ensureResourceExists(ctx context.Context, c *client.ResourceClient) error {
	log.Printf("Creating ThirdPartyResource %s", smith.TemplateResourceName)
	res := &smith.ThirdPartyResource{}
	err := c.Create(ctx, smith.ThirdPartyResourceAPIVersion, "", "thirdpartyresources", &smith.ThirdPartyResource{
		TypeMeta: smith.TypeMeta{
			Kind:       "ThirdPartyResource",
			APIVersion: smith.ThirdPartyResourceAPIVersion,
		},
		ObjectMeta: smith.ObjectMeta{
			Name: smith.TemplateResourceName,
		},
		Description: "Smith resource manager",
		Versions: []smith.APIVersion{
			{Name: smith.TemplateResourceVersion},
		},
	}, res)
	if err != nil {
		log.Printf("%#v", err)
		if !client.IsConflict(err) {
			return fmt.Errorf("failed to create ThirdPartyResource: %v", err)
		}
		log.Printf("ThirdPartyResource %s already exists", smith.TemplateResourceName)
	} else {
		log.Printf("ThirdPartyResource %s created: %+v", smith.TemplateResourceName, res)
	}
	return nil
}
