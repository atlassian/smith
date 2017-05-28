package resources

import "k8s.io/client-go/discovery"

// TODO implement caching and invalidation

type CachedDiscoveryClient struct {
	discovery.DiscoveryInterface
}

// Fresh returns true if no cached data was used that had been retrieved before the instantiation.
func (c *CachedDiscoveryClient) Fresh() bool {
	return true
}

// Invalidate enforces that no cached data is used in the future that is older than the current time.
func (c *CachedDiscoveryClient) Invalidate() {
}
