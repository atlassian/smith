package main

import (
	"context"
	"flag"
	"fmt"
	_ "net/http/pprof" // This is here to avoid adding pprof handler in app package. It may not be always desired.
	"os"

	"github.com/atlassian/smith/cmd/smith/app"
	"github.com/atlassian/smith/pkg/controller"
)

func main() {
	if err := run(); err != nil && err != context.Canceled && err != context.DeadlineExceeded {
		fmt.Fprintf(os.Stderr, "%#v\n", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()
	app.CancelOnInterrupt(ctx, cancelFunc)

	return runWithContext(ctx)
}

func runWithContext(ctx context.Context) error {
	controllers := []controller.Constructor{
		&app.BundleControllerConstructor{},
	}
	a, err := app.NewFromFlags(controllers, flag.CommandLine, os.Args[1:])
	if err != nil {
		return err
	}
	return a.Run(ctx)
}
