package sleeper

import (
	"context"
	"time"

	sleeper_v1 "github.com/atlassian/smith/examples/sleeper/pkg/apis/sleeper/v1"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

const (
	ResyncPeriod = 20 * time.Minute
)

type App struct {
	Logger     *zap.Logger
	RestConfig *rest.Config
	Namespace  string
}

func (a *App) Run(ctx context.Context) error {
	sClient, err := Client(a.RestConfig)
	if err != nil {
		return err
	}

	// Create an Informer for Sleeper objects
	sleeperInformer := a.sleeperInformer(ctx, sClient)

	// Run until a signal to stop
	sleeperInformer.Run(ctx.Done())
	return ctx.Err()
}

func (a *App) sleeperInformer(ctx context.Context, sClient rest.Interface) cache.SharedInformer {
	sleeperInf := cache.NewSharedInformer(
		cache.NewListWatchFromClient(sClient, sleeper_v1.SleeperResourcePlural, a.Namespace, fields.Everything()),
		&sleeper_v1.Sleeper{}, ResyncPeriod)

	eh := &EventHandler{
		ctx:    ctx,
		logger: a.Logger,
		client: sClient,
	}

	sleeperInf.AddEventHandler(eh)

	return sleeperInf
}
