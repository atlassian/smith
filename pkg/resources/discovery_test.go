package resources

import "k8s.io/client-go/discovery"

var (
	_ discovery.CachedDiscoveryInterface = &CachedDiscoveryClient{}
)
