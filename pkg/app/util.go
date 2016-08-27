package app

import (
	"context"
	"math"
	"time"

	"github.com/cenk/backoff"
)

func isCtxDone(err error) bool {
	return err == context.Canceled || err == context.DeadlineExceeded
}

func retryUntilSuccessOrDone(ctx context.Context, f func() error, handler func(error) bool) error {
	b := backoff.NewExponentialBackOff()
	b.MaxElapsedTime = time.Duration(math.MaxInt64)

	t := backoff.NewTicker(b)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			err := f()
			if err == nil || isCtxDone(err) {
				return err
			}
			if handler(err) {
				return err
			}

		}
	}
}
