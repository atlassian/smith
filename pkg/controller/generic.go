package controller

import (
	"context"
	"time"

	"github.com/ash2k/stager"
	"github.com/atlassian/smith/pkg/store"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

const (
	// Work queue deduplicates scheduled keys. This is the period it waits for duplicate keys before letting the work
	// to be dequeued.
	workDeduplicationPeriod = 50 * time.Millisecond
)

type Generic struct {
	logger      *zap.Logger
	queue       workQueue
	workers     int
	multi       *store.Multi
	Controllers map[schema.GroupVersionKind]Interface
	Informers   map[schema.GroupVersionKind]cache.SharedIndexInformer
}

func NewGeneric(config *Config, logger *zap.Logger, queue workqueue.RateLimitingInterface, workers int, constructors ...Constructor) (*Generic, error) {
	controllers := make(map[schema.GroupVersionKind]Interface, len(constructors))
	informers := make(map[schema.GroupVersionKind]cache.SharedIndexInformer)
	multi := store.NewMulti()
	wq := workQueue{
		queue: queue,
		workDeduplicationPeriod: workDeduplicationPeriod,
	}
	for _, constr := range constructors {
		descr := constr.Describe()
		if _, ok := controllers[descr.Gvk]; ok {
			return nil, errors.Errorf("duplicate controller for GVK %s", descr.Gvk)
		}
		iface, err := constr.New(config, &Context{
			Informers:   informers,
			Controllers: controllers,
			WorkQueue:   wq.NewQueueForGvk(descr.Gvk),
		})
		if err != nil {
			return nil, errors.Wrapf(err, "failed to construct controller for GVK %s", descr.Gvk)
		}
		inf, ok := informers[descr.Gvk]
		if !ok {
			return nil, errors.Errorf("controller for GVK %s should have registered an informer for that GVK", descr.Gvk)
		}
		err = multi.AddInformer(descr.Gvk, inf)
		if err != nil {
			return nil, errors.Errorf("failed to register informer for GVK %s in multistore", descr.Gvk)
		}
		controllers[descr.Gvk] = iface
	}
	return &Generic{
		logger:      logger,
		queue:       wq,
		workers:     workers,
		multi:       multi,
		Controllers: controllers,
		Informers:   informers,
	}, nil
}

func (g *Generic) Run(ctx context.Context) {
	// Stager will perform ordered, graceful shutdown
	stgr := stager.New()
	defer stgr.Shutdown()

	// Stage: start all informers then wait on them
	stage := stgr.NextStage()
	for _, inf := range g.Informers {
		stage.StartWithChannel(inf.Run)
	}
	g.logger.Info("Waiting for informers to sync")
	for _, inf := range g.Informers {
		if !cache.WaitForCacheSync(ctx.Done(), inf.HasSynced) {
			return
		}
	}
	g.logger.Info("Informers synced")

	// Stage: start all controllers
	stage = stgr.NextStage()
	for _, c := range g.Controllers {
		stage.StartWithContext(c.Run)
	}

	// Stage: start workers
	stage = stgr.NextStage()
	defer g.queue.ShutDown()
	for i := 0; i < g.workers; i++ {
		stage.Start(g.worker)
	}

	<-ctx.Done()
}
