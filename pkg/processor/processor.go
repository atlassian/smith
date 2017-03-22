package processor

import (
	"context"
	"sync"
	"time"

	"github.com/atlassian/smith"

	"github.com/cenk/backoff"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

type ReadyChecker interface {
	IsReady(*unstructured.Unstructured) (bool, error)
}

type BackOffFactory func() backoff.BackOff

type workerRef struct {
	namespace  string
	bundleName string
}

type BundleProcessor struct {
	ctx          context.Context
	backoff      BackOffFactory
	bundleClient *rest.RESTClient
	clients      dynamic.ClientPool
	rc           ReadyChecker
	scheme       *runtime.Scheme
	wg           sync.WaitGroup // tracks number of Goroutines running rebuildLoop()

	lock    sync.RWMutex // protects fields below
	workers map[workerRef]*worker
}

// New creates a new bundle processor.
// Instances are safe for concurrent use.
func New(ctx context.Context, bundleClient *rest.RESTClient, clients dynamic.ClientPool, rc ReadyChecker, scheme *runtime.Scheme) *BundleProcessor {
	return &BundleProcessor{
		ctx:          ctx,
		backoff:      exponentialBackOff,
		bundleClient: bundleClient,
		clients:      clients,
		rc:           rc,
		scheme:       scheme,
		workers:      make(map[workerRef]*worker),
	}
}

func (bp *BundleProcessor) Join() {
	bp.wg.Wait()
}

// Rebuild schedules a rebuild of the bundle.
// Note that the bundle object and/or resources in the bundle may be mutated asynchronously so the
// calling code should do a proper deep copy if the object is still needed.
func (bp *BundleProcessor) Rebuild(tpl *smith.Bundle) {
	//log.Printf("Rebuilding the bundle %#v", tpl)
	ref := workerRef{namespace: tpl.Metadata.Namespace, bundleName: tpl.Metadata.Name}
	bp.lock.Lock()
	defer bp.lock.Unlock()
	wrk := bp.workers[ref]
	if wrk == nil {
		wrk = &worker{
			bp:        bp,
			bundle:    tpl,
			bo:        bp.backoff(),
			workerRef: ref,
		}
		bp.workers[ref] = wrk
		bp.wg.Add(1)
		go wrk.rebuildLoop()
	} else {
		wrk.bundle = tpl
	}
}

func exponentialBackOff() backoff.BackOff {
	b := backoff.NewExponentialBackOff()
	b.InitialInterval = 2 * time.Second
	b.MaxElapsedTime = 5 * time.Second
	//b.MaxElapsedTime = time.Duration(math.MaxInt64)
	return b
}
