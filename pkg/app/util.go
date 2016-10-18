package app

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/cenk/backoff"

	"github.com/atlassian/smith/pkg/client"
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

// splitTprName splits TPR's name into resource name and group name.
// e.g. "postgresql-resource.smith-sql.atlassian.com" is split into "postgresqlresources" and "smith-sql.atlassian.com".
// See https://github.com/kubernetes/kubernetes/blob/master/docs/design/extending-api.md
// See k8s.io/pkg/api/meta/restmapper.go:147 KindToResource()
func splitTprName(name string) (string, string) {
	pos := strings.IndexByte(name, '.')
	if pos == -1 || pos == 0 {
		panic(fmt.Errorf("invalid resource name: %q", name))
	}
	resourcePath := strings.Replace(name[:pos], "-", "", -1)
	return client.ResourceKindToPath(resourcePath), name[pos+1:]
}
