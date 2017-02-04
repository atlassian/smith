package tprattribute

import (
	"context"
	"log"
	"time"

	"k8s.io/client-go/rest"
)

type SleeperEventHandler struct {
	ctx    context.Context
	client *rest.RESTClient
}

func (h *SleeperEventHandler) OnAdd(obj interface{}) {
	in := *obj.(*Sleeper)
	log.Printf("[Sleeper] %s/%s was added. Setting Status to %q and falling asleep for %d seconds... ZZzzzz", in.Metadata.Namespace, in.Metadata.Name, SLEEPING, in.Spec.SleepFor)
	in.Status.State = SLEEPING
	err := h.client.Put().
		Namespace(in.Metadata.Namespace).
		Resource(SleeperResourcePath).
		Name(in.Metadata.Name).
		Body(&in).
		Do().
		Into(&in)
	if err != nil {
		log.Printf("[Sleeper] Status update for %s/%s failed: %v", in.Metadata.Namespace, in.Metadata.Name, err)
	}
	go func() {
		select {
		case <-h.ctx.Done():
			return
		case <-time.After(time.Duration(in.Spec.SleepFor) * time.Second):
			log.Printf("[Sleeper] %s Updating %s/%s Status to %q", in.Spec.WakeupMessage, in.Metadata.Namespace, in.Metadata.Name, AWAKE)
			in.Status.State = AWAKE
			in.Status.Message = in.Spec.WakeupMessage
			err := h.client.Put().
				Namespace(in.Metadata.Namespace).
				Resource(SleeperResourcePath).
				Name(in.Metadata.Name).
				Body(&in).
				Do().
				Error()
			if err != nil {
				log.Printf("[Sleeper] Status update for %s/%s failed: %v", in.Metadata.Namespace, in.Metadata.Name, err)
			}

		}
	}()
}

func (h *SleeperEventHandler) OnUpdate(oldObj, newObj interface{}) {
}

func (h *SleeperEventHandler) OnDelete(obj interface{}) {
}
