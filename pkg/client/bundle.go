package client

import (
	"time"

	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"

	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/tools/cache"
)

func BundleInformer(bundleClient cache.Getter, namespace string, resyncPeriod time.Duration) cache.SharedIndexInformer {
	return cache.NewSharedIndexInformer(
		cache.NewListWatchFromClient(bundleClient, smith_v1.BundleResourcePlural, namespace, fields.Everything()),
		&smith_v1.Bundle{},
		resyncPeriod,
		cache.Indexers{})
}
