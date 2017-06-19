package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/atlassian/smith/pkg/app"
	"github.com/atlassian/smith/pkg/resources"

	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
)

const (
	defaultResyncPeriod = 20 * time.Minute
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
	a := app.App{
		Workers: 2,
	}

	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	fs.BoolVar(&a.DisablePodPreset, "disable-pod-preset", false, "Disable PodPreset support")
	scUrl := fs.String("service-catalog-url", "", "Service Catalog API server URL")
	fs.DurationVar(&a.ResyncPeriod, "resync-period", defaultResyncPeriod, "Resync period for informers")
	fs.StringVar(&a.Namespace, "namespace", meta_v1.NamespaceAll, "Namespace to use. All namespaces are used if empty string or omitted")
	fs.Parse(os.Args[1:]) // nolint: gas

	config, err := resources.ConfigFromEnv()
	if err != nil {
		config, err = rest.InClusterConfig()
		if err != nil {
			return err
		}
	}
	config.UserAgent = "smith/" + Version + "/" + GitCommit
	a.RestConfig = config
	if *scUrl != "" {
		scConfig := *config // shallow copy
		scConfig.Host = *scUrl
		a.ServiceCatalogConfig = &scConfig
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
