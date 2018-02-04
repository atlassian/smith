package controller

import (
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/smith/pkg/util/logz"

	"go.uber.org/zap"
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
		namespace, name, err := cache.SplitMetaNamespaceKey(tombstone.Key)
		if err != nil {
			c.Logger.
				With(zap.Error(err)).
				Sugar().Errorf("Failed to split key %q", tombstone.Key)
			c.Logger.Sugar().Infof("Rebuilding deleted Bundle (tombstone) with key=%q", tombstone.Key)
		} else {
			c.Logger.Info("Rebuilding deleted Bundle (tombstone)", logz.NamespaceName(namespace), logz.BundleName(name))
		}
		c.enqueueKey(tombstone.Key)
		return
	}
	c.Logger.Info("Rebuilding Bundle because it was deleted", logz.Namespace(bundle), logz.Bundle(bundle))
	c.enqueue(bundle)
}
