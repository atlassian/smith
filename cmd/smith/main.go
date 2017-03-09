package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/atlassian/smith/pkg/app"
	"github.com/atlassian/smith/pkg/resources"

	"k8s.io/client-go/rest"
)

func main() {
	if err := run(); err != nil && err != context.Canceled && err != context.DeadlineExceeded {
		log.Fatalln(err)
	}
}

func run() error {
	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()
	cancelOnInterrupt(ctx, cancelFunc)

	return runWithContext(ctx)
}

func runWithContext(ctx context.Context) error {
	config, err := resources.ConfigFromEnv()
	if err != nil {
		config, err = rest.InClusterConfig()
		if err != nil {
			return err
		}
	}
	config.UserAgent = "smith/" + Version + "/" + GitCommit

	return runWithConfig(ctx, config)
}

func runWithConfig(ctx context.Context, config *rest.Config) error {
	a := app.App{
		RestConfig: config,
	}
	return a.Run(ctx)
}

// cancelOnInterrupt calls f when os.Interrupt or SIGTERM is received.
// It ignores subsequent interrupts on purpose - program should exit correctly after the first signal.
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
