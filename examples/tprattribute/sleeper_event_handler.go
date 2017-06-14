package tprattribute

import (
	"context"
	"log"
	"time"

	"github.com/atlassian/smith"

	"k8s.io/client-go/rest"
)

type SleeperEventHandler struct {
	ctx      context.Context
	client   *rest.RESTClient
	deepCopy smith.DeepCopy
}

func (h *SleeperEventHandler) OnAdd(obj interface{}) {
	h.handle(obj)
}

func (h *SleeperEventHandler) OnUpdate(oldObj, newObj interface{}) {
	in := *newObj.(*Sleeper)
	if in.Status.State == New {
		h.handle(newObj)
	}
}

func (h *SleeperEventHandler) OnDelete(obj interface{}) {
}

func (h *SleeperEventHandler) handle(obj interface{}) {
	obj, err := h.deepCopy(obj)
	if err != nil {
		log.Printf("[Sleeper] Failed to deep copy %T: %v", obj, err)
		return
	}
	in := obj.(*Sleeper)
	log.Printf("[Sleeper] %s/%s was added/updated. Setting Status to %q and falling asleep for %d seconds... ZZzzzz", in.Namespace, in.Name, Sleeping, in.Spec.SleepFor)
	in.Status.State = Sleeping
	err = h.client.Put().
		Namespace(in.Namespace).
		Resource(SleeperResourcePath).
		Name(in.Name).
		Context(h.ctx).
		Body(in).
		Do().
		Into(in)
	if err != nil {
		log.Printf("[Sleeper] Status update for %s/%s failed: %v", in.Namespace, in.Name, err)
		return
	}
	go func() {
		select {
		case <-h.ctx.Done():
			return
		case <-time.After(time.Duration(in.Spec.SleepFor) * time.Second):
			log.Printf("[Sleeper] %s Updating %s/%s Status to %q", in.Spec.WakeupMessage, in.Namespace, in.Name, Awake)
			in.Status.State = Awake
			in.Status.Message = in.Spec.WakeupMessage
			err := h.client.Put().
				Namespace(in.Namespace).
				Resource(SleeperResourcePath).
				Name(in.Name).
				Context(h.ctx).
				Body(in).
				Do().
				Error()
			if err != nil {
				log.Printf("[Sleeper] Status update for %s/%s failed: %v", in.Namespace, in.Name, err)
			}

		}
	}()
}
