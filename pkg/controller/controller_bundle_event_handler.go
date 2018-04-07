package controller

import (
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/smith/pkg/util/logz"
	"k8s.io/client-go/tools/cache"
)

func (c *BundleController) onBundleAdd(obj interface{}) {
	bundle := obj.(*smith_v1.Bundle)
	c.Logger.Info("Rebuilding Bundle because it was added", logz.Namespace(bundle), logz.Bundle(bundle))
	c.enqueue(bundle)
}

func (c *BundleController) onBundleUpdate(oldObj, newObj interface{}) {
	bundle := newObj.(*smith_v1.Bundle)
	c.Logger.Info("Rebuilding Bundle because it was updated", logz.Namespace(bundle), logz.Bundle(bundle))
	c.enqueue(bundle)
}

func (c *BundleController) onBundleDelete(obj interface{}) {
	bundle, ok := obj.(*smith_v1.Bundle)
	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			c.Logger.Sugar().Errorf("Delete event with unrecognized object type: %T", obj)
			return
		}
		bundle, ok = tombstone.Obj.(*smith_v1.Bundle)
		if !ok {
			c.Logger.Sugar().Errorf("Delete tombstone with unrecognized object type: %T", tombstone.Obj)
			return
		}
	}
	c.Logger.Info("Rebuilding Bundle because it was deleted", logz.Namespace(bundle), logz.Bundle(bundle))
	c.enqueue(bundle)
}
