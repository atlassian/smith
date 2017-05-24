package processor

import (
	"context"
	"sync"
	"time"

	"github.com/atlassian/smith"

	"github.com/cenk/backoff"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

type BundleProcessor struct {
	workerConfig
	backoff      BackOffFactory
	incomingWork chan *smith.Bundle
}

// New creates a new bundle processor.
// Instances are safe for concurrent use.
func New(bundleClient *rest.RESTClient, scDynamic, clients dynamic.ClientPool, rc ReadyChecker, deepCopy smith.DeepCopy, store Store) *BundleProcessor {
	return &BundleProcessor{
		workerConfig: workerConfig{
			bundleClient: bundleClient,
			scDynamic:    scDynamic,
			clients:      clients,
			rc:           rc,
			deepCopy:     deepCopy,
			store:        store,
		},
		backoff:      exponentialBackOff,
		incomingWork: make(chan *smith.Bundle),
	}
}

func (bp *BundleProcessor) Run(ctx context.Context, done func()) {
	defer done()
	var wg sync.WaitGroup
	defer wg.Wait()
	workers := make(map[bundleRef]*worker)
	workRequests := make(chan workRequest)
	notifyRequests := make(chan notifyRequest)
	for {
		select {
		case <-ctx.Done():
			return
		case bundle := <-bp.incomingWork:
			ref := bundleRef{namespace: bundle.Namespace, bundleName: bundle.Name}
			wrk := workers[ref]
			if wrk == nil {
				wrk = &worker{
					bundleRef:      ref,
					workerConfig:   bp.workerConfig,
					bo:             bp.backoff(),
					workRequests:   workRequests,
					notifyRequests: notifyRequests,
					pendingBundle:  bundle,
				}
				workers[ref] = wrk
				wg.Add(1)
				go wrk.rebuildLoop(ctx, wg.Done)
			} else {
				wrk.setNeedsRebuild()
				wrk.pendingBundle = bundle
				if wrk.notify != nil {
					close(wrk.notify)
					wrk.notify = nil
				}
			}
		case req := <-workRequests:
			wrk := workers[req.bundleRef]
			if wrk.pendingBundle == nil {
				// no work is scheduled for worker
				close(req.work)
				delete(workers, req.bundleRef)
				break
			}
			wrk.resetNeedsRebuild()
			select {
			case <-ctx.Done():
				return
			case req.work <- wrk.pendingBundle:
				wrk.pendingBundle = nil
				wrk.notify = nil
			}
		case nrq := <-notifyRequests:
			wrk := workers[nrq.bundleRef]
			if wrk.pendingBundle == nil {
				// No pending work, let it sleep
				wrk.pendingBundle = nrq.bundle
				wrk.notify = nrq.notify
			} else {
				// Have pending work already, notify immediately
				close(nrq.notify)
			}
		}
	}
}

// Rebuild schedules a rebuild of the bundle.
// Note that the bundle object and/or resources in the bundle may be mutated asynchronously so the
// calling code should do a proper deep copy if the object is still needed.
func (bp *BundleProcessor) Rebuild(ctx context.Context, bundle *smith.Bundle) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case bp.incomingWork <- bundle:
		return nil
	}
}

func exponentialBackOff() backoff.BackOff {
	b := backoff.NewExponentialBackOff()
	b.InitialInterval = 2 * time.Second
	b.MaxElapsedTime = 5 * time.Second
	//b.MaxElapsedTime = time.Duration(math.MaxInt64)
	return b
}
