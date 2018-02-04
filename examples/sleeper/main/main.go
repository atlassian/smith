package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/atlassian/smith/examples/sleeper"
	"github.com/atlassian/smith/pkg/client"

	"go.uber.org/zap"
	"k8s.io/client-go/rest"
)

func main() {
	if err := run(); err != nil && err != context.Canceled && err != context.DeadlineExceeded {
		fmt.Fprintf(os.Stderr, "%#v", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()
	cancelOnInterrupt(ctx, cancelFunc)

	return runWithContext(ctx)
}

func runWithContext(ctx context.Context) error {
	configFileFrom := flag.String("client-config-from", "in-cluster",
		"Source of REST client configuration. 'in-cluster' (default), 'environment' and 'file' are valid options.")
	configFileName := flag.String("client-config-file-name", "",
		"Load REST client configuration from the specified Kubernetes config file. This is only applicable if --client-config-from=file is set.")
	configContext := flag.String("client-config-context", "",
		"Context to use for REST client configuration. This is only applicable if --client-config-from=file is set.")

	flag.Parse()

	config, err := client.LoadConfig(*configFileFrom, *configFileName, *configContext)
	if err != nil {
		return err
	}
	config.UserAgent = "sleeper-controller"

	return runWithConfig(ctx, config)
}

func runWithConfig(ctx context.Context, config *rest.Config) error {
	logger, err := zap.NewDevelopment()
	if err != nil {
		return err
	}
	a := sleeper.App{
		Logger:     logger,
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
