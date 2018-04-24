package bundlec

import (
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/smith/pkg/controller"
	"github.com/atlassian/smith/pkg/util/logz"
	"go.uber.org/zap"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
)

func (c *Controller) onResourceAdd(obj interface{}) {
	bundleName, namespace := getBundleNameAndNamespace(obj)
	logger := c.loggerForObj(obj)
	c.rebuildByName(logger, namespace, bundleName, "added", obj)
}

func (c *Controller) onResourceUpdate(oldObj, newObj interface{}) {
	oldBundleName, oldNamespace := getBundleNameAndNamespace(oldObj)

	newBundleName, newNamespace := getBundleNameAndNamespace(newObj)

	if oldBundleName != newBundleName { // changed controller of the object
		logger := c.loggerForObj(oldObj)
		c.rebuildByName(logger, oldNamespace, oldBundleName, "updated", oldObj)
	}
	logger := c.loggerForObj(newObj)
	c.rebuildByName(logger, newNamespace, newBundleName, "updated", newObj)
}

func (c *Controller) onResourceDelete(obj interface{}) {
	logger := c.loggerForObj(obj)
	metaObj, ok := obj.(meta_v1.Object)
	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			logger.Sugar().Errorf("Delete event with unrecognized object type: %T", obj)
			return
		}
		metaObj, ok = tombstone.Obj.(meta_v1.Object)
		if !ok {
			logger.Sugar().Errorf("Delete tombstone with unrecognized object type: %T", tombstone.Obj)
			return
		}
		logger = c.loggerForObj(metaObj)
	}
	bundleName, namespace := getBundleNameAndNamespace(metaObj)
	if bundleName == "" { // No controller bundle found
		runtimeObj := metaObj.(runtime.Object)
		bundles, err := c.BundleStore.GetBundlesByObject(runtimeObj.GetObjectKind().GroupVersionKind().GroupKind(), namespace, metaObj.GetName())
		if err != nil {
			logger.Error("Failed to get Bundles by object", zap.Error(err))
			return
		}
		for _, bundle := range bundles {
			c.rebuildByName(logger, namespace, bundle.Name, "deleted", metaObj)
		}
	} else {
		c.rebuildByName(logger, namespace, bundleName, "deleted", metaObj)
	}
}

// This method may be called with empty bundleName.
func (c *Controller) rebuildByName(logger *zap.Logger, namespace, bundleName, addUpdateDelete string, obj interface{}) {
	if bundleName == "" {
		return
	}
	logger.
		With(logz.BundleName(bundleName)).
		Sugar().Infof("Rebuilding Bundle because object was %s", addUpdateDelete)
	c.WorkQueue.Add(controller.QueueKey{
		Namespace: namespace,
		Name:      bundleName,
	})
}

func getBundleNameAndNamespace(obj interface{}) (string, string) {
	var bundleName string
	meta := obj.(meta_v1.Object)
	ref := meta_v1.GetControllerOf(meta)
	if ref != nil && ref.APIVersion == smith_v1.BundleResourceGroupVersion && ref.Kind == smith_v1.BundleResourceKind {
		bundleName = ref.Name
	}
	return bundleName, meta.GetNamespace()
}
