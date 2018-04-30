package sleeper

import (
	"context"
	"time"

	ctrlLogz "github.com/atlassian/ctrl/logz"
	sleeper_v1 "github.com/atlassian/smith/examples/sleeper/pkg/apis/sleeper/v1"
	"go.uber.org/zap"
	api_errors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/rest"
)

type SleeperEventHandler struct {
	ctx    context.Context
	logger *zap.Logger
	client rest.Interface
}

func (h *SleeperEventHandler) OnAdd(obj interface{}) {
	h.handle(obj)
}

func (h *SleeperEventHandler) OnUpdate(oldObj, newObj interface{}) {
	in := *newObj.(*sleeper_v1.Sleeper)
	if in.Status.State == sleeper_v1.New {
		h.handle(newObj)
	}
}

func (h *SleeperEventHandler) OnDelete(obj interface{}) {
}

func (h *SleeperEventHandler) handle(obj interface{}) {
	in := obj.(*sleeper_v1.Sleeper).DeepCopy()
	msg := in.Spec.WakeupMessage
	logger := h.logger.With(ctrlLogz.Namespace(in), ctrlLogz.Object(in))
	logger.Sugar().Infof("Sleeper was added/updated. Setting Status to %q and falling asleep for %d seconds... ZZzzzz", sleeper_v1.Sleeping, in.Spec.SleepFor)
	err := h.retryUpdate(in, sleeper_v1.Sleeping, "")
	if err != nil {
		logger.Error("Status update failed", zap.Error(err))
		return
	}
	go func() {
		select {
		case <-h.ctx.Done():
			return
		case <-time.After(time.Duration(in.Spec.SleepFor) * time.Second):
			logger.Sugar().Infof("%s Updating Sleeper Status to %q", in.Spec.WakeupMessage, sleeper_v1.Awake)
			err = h.retryUpdate(in, sleeper_v1.Awake, msg)
			if err != nil {
				logger.Error("Status update failed", zap.Error(err))
			}
		}
	}()
}

func (h *SleeperEventHandler) retryUpdate(sleeper *sleeper_v1.Sleeper, state sleeper_v1.SleeperState, message string) error {
	for {
		sleeper.Status.State = state
		sleeper.Status.Message = message
		err := h.client.Put().
			Context(h.ctx).
			Namespace(sleeper.Namespace).
			Resource(sleeper_v1.SleeperResourcePlural).
			Name(sleeper.Name).
			Body(sleeper).
			Do().
			Into(sleeper)
		if api_errors.IsConflict(err) {
			err = h.client.Get().
				Context(h.ctx).
				Namespace(sleeper.Namespace).
				Resource(sleeper_v1.SleeperResourcePlural).
				Name(sleeper.Name).
				Do().
				Into(sleeper)
			if err != nil {
				return err
			}
			continue
		}
		return err
	}
}
