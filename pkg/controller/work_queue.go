package controller

import (
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/util/workqueue"
)

type gvkQueueKey struct {
	gvk schema.GroupVersionKind
	QueueKey
}

func (g *gvkQueueKey) String() string {
	return fmt.Sprintf("%s, Ns=%s, N=%s", g.gvk, g.Namespace, g.Name)
}

// workQueue is a type safe wrapper around workqueue.RateLimitingInterface.
type workQueue struct {
	// Objects that need to be synced.
	queue                   workqueue.RateLimitingInterface
	workDeduplicationPeriod time.Duration
}

func (q *workQueue) ShutDown() {
	q.queue.ShutDown()
}

func (q *workQueue) Get() (item gvkQueueKey, shutdown bool) {
	i, s := q.queue.Get()
	if s {
		return gvkQueueKey{}, true
	}
	return i.(gvkQueueKey), false
}

func (q *workQueue) Done(item gvkQueueKey) {
	q.queue.Done(item)
}

func (q *workQueue) Forget(item gvkQueueKey) {
	q.queue.Forget(item)
}

func (q *workQueue) NumRequeues(item gvkQueueKey) int {
	return q.queue.NumRequeues(item)
}

func (q *workQueue) AddRateLimited(item gvkQueueKey) {
	q.queue.AddRateLimited(item)
}

func (q *workQueue) AddAfter(item gvkQueueKey, duration time.Duration) {
	q.queue.AddAfter(item, duration)
}

func (q *workQueue) NewQueueForGvk(gvk schema.GroupVersionKind) *gvkQueue {
	return &gvkQueue{
		queue: q.queue,
		gvk:   gvk,
		workDeduplicationPeriod: q.workDeduplicationPeriod,
	}
}

type gvkQueue struct {
	queue                   workqueue.RateLimitingInterface
	gvk                     schema.GroupVersionKind
	workDeduplicationPeriod time.Duration
}

func (q *gvkQueue) Add(item QueueKey) {
	q.queue.AddAfter(gvkQueueKey{
		gvk:      q.gvk,
		QueueKey: item,
	}, q.workDeduplicationPeriod)
}
