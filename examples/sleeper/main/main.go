package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	ctrlApp "github.com/atlassian/ctrl/app"
	ctrlLogz "github.com/atlassian/ctrl/logz"
	"github.com/atlassian/smith/examples/sleeper"
	"github.com/atlassian/smith/pkg/client"
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
	ctrlApp.CancelOnInterrupt(ctx, cancelFunc)

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
	a := sleeper.App{
		Logger:     ctrlLogz.LoggerStr("debug", "console"),
		RestConfig: config,
	}
	return a.Run(ctx)
}
