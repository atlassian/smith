package controller

import (
	"time"

	"github.com/atlassian/smith/pkg/util/logz"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	api_errors "k8s.io/apimachinery/pkg/api/errors"
)

const (
	// maxRetries is the number of times a Bundle object will be retried before it is dropped out of the queue.
	// With the current rate-limiter in use (5ms*2^(maxRetries-1)) the following numbers represent the times
	// an object is going to be requeued:
	//
	// 5ms, 10ms, 20ms, 40ms, 80ms, 160ms, 320ms, 640ms, 1.3s, 2.6s, 5.1s, 10.2s, 20.4s, 41s, 82s
	maxRetries = 15
)

func (g *Generic) worker() {
	for g.processNextWorkItem() {
	}
}

func (g *Generic) processNextWorkItem() bool {
	key, quit := g.queue.Get()
	if quit {
		return false
	}
	defer g.queue.Done(key)

	holder := g.Controllers[key.gvk]
	logger := g.logger.With(logz.NamespaceName(key.Namespace), holder.ZapNameField(key.Name))

	retriable, err := g.processKey(logger, holder.Cntrlr, key)
	g.handleErr(logger, retriable, err, key)

	return true
}

func (g *Generic) handleErr(logger *zap.Logger, retriable bool, err error, key gvkQueueKey) {
	if err == nil {
		g.queue.Forget(key)
		return
	}
	if retriable && g.queue.NumRequeues(key) < maxRetries {
		logger.Info("Error syncing object", zap.Error(err))
		g.queue.AddRateLimited(key)
		return
	}

	logger.Info("Dropping object out of the queue", zap.Error(err))
	g.queue.Forget(key)
}

func (g *Generic) processKey(logger *zap.Logger, cntrlr Interface, key gvkQueueKey) (bool /*retriable*/, error) {
	obj, exists, err := g.multi.Get(key.gvk, key.Namespace, key.Name)
	if err != nil {
		return false, errors.Wrapf(err, "failed to get object by key %s", key)
	}
	if !exists {
		logger.Debug("Object not in cache. Was deleted?")
		return false, nil
	}
	startTime := time.Now()
	logger.Info("Started syncing")
	retriable, err := cntrlr.Process(&ProcessContext{
		Logger: logger,
		Object: obj,
	})
	msg := ""
	if err != nil && api_errors.IsConflict(errors.Cause(err)) {
		msg = " (conflict)"
		err = nil
	}
	logger.Sugar().Infof("Synced in %v%s", time.Since(startTime), msg)
	return retriable, err
}
