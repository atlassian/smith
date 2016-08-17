package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/ash2k/smith"
	"github.com/ash2k/smith/pkg/client"
)

const (
	ResourceName                 = "resource.smith.ash2k.com"
	ResourceVersion              = "v1"
	ResourceGroupVersion         = ResourceName + "/" + ResourceVersion
	ThirdPartyResourceAPIVersion = "extensions/v1beta1"
	// TODO make it work with all namespaces
	Namespace = "default"
)

func main() {
	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()
	cancelOnInterrupt(ctx, cancelFunc)

	if err := realMain(ctx); err != nil && err != context.Canceled && err != context.DeadlineExceeded {
		log.Fatalln(err)
	}
}

func realMain(ctx context.Context) error {
	c, err := client.NewInCluster()
	if err != nil {
		return err
	}
	c.Agent = "smith/" + Version + "/" + GitCommit
	if err = ensureResourceExists(ctx, c); err != nil {
		return err
	}

	//c.List(ctx, ResourceGroupVersion, Namespace, )

	<-ctx.Done()
	return ctx.Err()
}

func ensureResourceExists(ctx context.Context, c *client.ResourceClient) error {
	log.Printf("Creating ThirdPartyResource %s", ResourceName)
	res := &smith.ThirdPartyResource{}
	err := c.Create(ctx, ThirdPartyResourceAPIVersion, "", "thirdpartyresources", &smith.ThirdPartyResource{
		TypeMeta: smith.TypeMeta{
			Kind:       "ThirdPartyResource",
			APIVersion: ThirdPartyResourceAPIVersion,
		},
		ObjectMeta: smith.ObjectMeta{
			Name: ResourceName,
		},
		Description: "Smith resource manager",
		Versions: []smith.APIVersion{
			{Name: ResourceVersion},
		},
	}, res)
	if err != nil {
		log.Printf("%#v", err)
		if !client.IsConflict(err) {
			return fmt.Errorf("failed to create ThirdPartyResource: %v", err)
		}
		log.Printf("ThirdPartyResource %s already exists", ResourceName)
	} else {
		log.Printf("ThirdPartyResource %s created: %+v", ResourceName, res)
	}
	return nil
}

// cancelOnInterrupt calls f when os.Interrupt or SIGTERM is received.
func cancelOnInterrupt(ctx context.Context, f context.CancelFunc) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		select {
		case <-ctx.Done():
		case <-c:
			f()
		}
	}()
}
