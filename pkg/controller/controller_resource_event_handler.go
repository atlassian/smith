package controller

import (
	"log"

	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/smith/pkg/resources"

	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
)

func (c *BundleController) onResourceAdd(obj interface{}) {
	bundleName, namespace := getBundleNameAndNamespace(obj)
	c.rebuildByName(namespace, bundleName, "added", obj)
}

func (c *BundleController) onResourceUpdate(oldObj, newObj interface{}) {
	oldBundleName, oldNamespace := getBundleNameAndNamespace(oldObj)

	newBundleName, newNamespace := getBundleNameAndNamespace(newObj)

	if oldBundleName != newBundleName { // changed controller of the object
		c.rebuildByName(oldNamespace, oldBundleName, "updated", oldObj)
	}
	c.rebuildByName(newNamespace, newBundleName, "updated", newObj)
}

func (c *BundleController) onResourceDelete(obj interface{}) {
	metaObj, ok := obj.(meta_v1.Object)
	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			log.Printf("[REH] Delete event with unrecognized object type: %T", obj)
			return
		}
		metaObj, ok = tombstone.Obj.(meta_v1.Object)
		if !ok {
			log.Printf("[REH] Delete tombstone with unrecognized object type: %T", tombstone.Obj)
			return
		}
	}
	bundleName, namespace := getBundleNameAndNamespace(metaObj)
	if bundleName == "" { // No controller bundle found
		runtimeObj := metaObj.(runtime.Object)
		bundles, err := c.bundleStore.GetBundlesByObject(runtimeObj.GetObjectKind().GroupVersionKind().GroupKind(), namespace, metaObj.GetName())
		if err != nil {
			log.Printf("[REH] Failed to get bundles by object: %v", err)
			return
		}
		for _, bundle := range bundles {
			c.rebuildByName(namespace, bundle.Name, "deleted", metaObj)
		}
	} else {
		c.rebuildByName(namespace, bundleName, "deleted", metaObj)
	}
}

func (c *BundleController) rebuildByName(namespace, bundleName, addUpdateDelete string, obj interface{}) {
	if len(bundleName) == 0 {
		return
	}
	// TODO print GVK
	log.Printf("[REH][%s/%s] Rebuilding bundle because object %s was %s",
		namespace, bundleName, obj.(meta_v1.Object).GetName(), addUpdateDelete)
	c.enqueue(&smith_v1.Bundle{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      bundleName,
			Namespace: namespace,
		},
	})
}

func getBundleNameAndNamespace(obj interface{}) (string, string) {
	var bundleName string
	meta := obj.(meta_v1.Object)
	ref := resources.GetControllerOf(meta)
	if ref != nil && ref.APIVersion == smith_v1.BundleResourceGroupVersion && ref.Kind == smith_v1.BundleResourceKind {
		bundleName = ref.Name
	}
	return bundleName, meta.GetNamespace()
}
