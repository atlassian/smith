package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/net/context"
)

const (
	ResourceName                 = "resource.smith.ash2k.com"
	ThirdPartyResourceAPIVersion = "extensions/v1beta1"
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
	c, err := NewInCluster()
	if err != nil {
		return err
	}
	if err = ensureResourceExists(ctx, c); err != nil {
		return err
	}

	<-ctx.Done()
	return ctx.Err()
}

func ensureResourceExists(ctx context.Context, c *ResourceClient) error {
	log.Printf("Creating ThirdPartyResource %s", ResourceName)
	res := &ThirdPartyResource{}
	err := c.Create(ctx, ThirdPartyResourceAPIVersion, "", "thirdpartyresources", &ThirdPartyResource{
		TypeMeta: TypeMeta{
			Kind:       "ThirdPartyResource",
			APIVersion: ThirdPartyResourceAPIVersion,
		},
		ObjectMeta: ObjectMeta{
			Name: ResourceName,
		},
		Description: "Smith resource manager",
		Versions: []APIVersion{
			{Name: "v1"},
		},
	}, res)
	if err != nil {
		log.Printf("%#v", err)
		if !IsConflict(err) {
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
