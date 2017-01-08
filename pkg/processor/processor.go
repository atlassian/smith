package processor

import (
	"context"
	"log"
	"math"
	"sync"
	"time"

	"github.com/atlassian/smith"

	"github.com/cenk/backoff"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

type ReadyChecker interface {
	IsReady(*smith.Resource) (bool, error)
}

type BackOffFactory func() backoff.BackOff

type workerRef struct {
	namespace string
	name      string
}

type TemplateProcessor struct {
	ctx            context.Context
	backoff        BackOffFactory
	templateClient *rest.RESTClient
	clients        dynamic.ClientPool
	rc             ReadyChecker
	wg             sync.WaitGroup // tracks number of Goroutines running rebuildLoop()

	lock    sync.RWMutex // protects fields below
	workers map[workerRef]*worker
}

// New creates a new template processor.
// Instances are safe for concurrent use.
func New(ctx context.Context, templateClient *rest.RESTClient, clients dynamic.ClientPool, rc ReadyChecker) *TemplateProcessor {
	return &TemplateProcessor{
		ctx:            ctx,
		backoff:        exponentialBackOff,
		templateClient: templateClient,
		clients:        clients,
		rc:             rc,
		workers:        make(map[workerRef]*worker),
	}
}

func (tp *TemplateProcessor) Join() {
	tp.wg.Wait()
}

// Rebuild schedules a rebuild of the template.
// Note that the template object and/or resources in the template may be mutated asynchronously so the
// calling code should do a proper deep copy if the object is still needed.
func (tp *TemplateProcessor) Rebuild(tpl *smith.Template) {
	log.Printf("Rebuilding the template %#v", tpl)
	tp.rebuildInternal(tpl.Metadata.Namespace, tpl.Metadata.Name, tpl)
}

func (tp *TemplateProcessor) RebuildByName(namespace, name string) {
	log.Printf("Rebuilding the template %s/%s", namespace, name)
	tp.rebuildInternal(namespace, name, nil)
}

func (tp *TemplateProcessor) rebuildInternal(namespace, name string, tpl *smith.Template) {
	ref := workerRef{namespace: namespace, name: name}
	tp.lock.Lock()
	defer tp.lock.Unlock()
	wrk := tp.workers[ref]
	if wrk == nil {
		wrk = &worker{
			tp:           tp,
			template:     tpl,
			bo:           tp.backoff(),
			workerRef:    ref,
			needsRebuild: true,
		}
		tp.workers[ref] = wrk
		tp.wg.Add(1)
		go wrk.rebuildLoop()
	} else {
		wrk.template = tpl
		wrk.needsRebuild = true
	}
}

func exponentialBackOff() backoff.BackOff {
	b := backoff.NewExponentialBackOff()
	b.InitialInterval = 2 * time.Second
	b.MaxElapsedTime = time.Duration(math.MaxInt64)
	return b
}
