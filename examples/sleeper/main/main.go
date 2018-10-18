package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	ctrlApp "github.com/atlassian/ctrl/app"
	"github.com/atlassian/ctrl/options"
	"github.com/atlassian/smith/examples/sleeper"
	"k8s.io/client-go/rest"
)

func main() {
	if err := run(); err != nil && err != context.Canceled && err != context.DeadlineExceeded {
		fmt.Fprintf(os.Stderr, "%#v", err) // nolint: gas, errcheck
		os.Exit(1)
	}
}

func run() error {
	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()
	ctrlApp.CancelOnInterrupt(ctx, cancelFunc)

	return runWithContext(ctx)
}

func runWithContext(ctx context.Context) error {
	var restClientOpts options.RestClientOptions
	options.BindRestClientFlags(&restClientOpts, flag.CommandLine)

	flag.Parse()

	config, err := options.LoadRestClientConfig("sleeper-controller", restClientOpts)
	if err != nil {
		return err
	}

	return runWithConfig(ctx, config)
}

func runWithConfig(ctx context.Context, config *rest.Config) error {
	a := sleeper.App{
		Logger: options.LoggerFromOptions(options.LoggerOptions{
			LogLevel:    "debug",
			LogEncoding: "console",
		}),
		RestConfig: config,
	}
	return a.Run(ctx)
}
