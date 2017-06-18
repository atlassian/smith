package controller

import (
	"log"

	"github.com/atlassian/smith"

	"k8s.io/client-go/tools/cache"
)

func (c *BundleController) onBundleAdd(obj interface{}) {
	bundle := obj.(*smith.Bundle)
	log.Printf("[%s/%s] Rebuilding bundle because it was added", bundle.Namespace, bundle.Name)
	c.enqueue(bundle)

}

func (c *BundleController) onBundleUpdate(oldObj, newObj interface{}) {
	bundle := newObj.(*smith.Bundle)
	log.Printf("[%s/%s] Rebuilding bundle because it was updated", bundle.Namespace, bundle.Name)
	c.enqueue(bundle)
}

func (c *BundleController) onBundleDelete(obj interface{}) {
	bundle, ok := obj.(*smith.Bundle)
	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			log.Printf("Delete event with unrecognized object type: %T", obj)
			return
		}
		log.Printf("[%s] Rebuilding deleted bundle (tombstone)", tombstone.Key)
		c.enqueueKey(tombstone.Key)
		return
	}
	log.Printf("[%s/%s] Rebuilding bundle because it was deleted", bundle.Namespace, bundle.Name)
	c.enqueue(bundle)
}
