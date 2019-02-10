package sc

import (
	"context"
	"testing"

	"github.com/atlassian/smith/it"
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestServiceCatalog(t *testing.T) {
	t.Parallel()
	bundle := &smith_v1.Bundle{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       smith_v1.BundleResourceKind,
			APIVersion: smith_v1.BundleResourceGroupVersion,
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "bundle-cs",
		},
		Spec: smith_v1.BundleSpec{
			Resources: []smith_v1.Resource{
				{
					Name: smith_v1.ResourceName(serviceInstance1name),
					Spec: smith_v1.ResourceSpec{
						Object: serviceInstance1(),
					},
				},
				{
					Name: smith_v1.ResourceName(serviceBinding1name),
					References: []smith_v1.Reference{
						{
							Resource: smith_v1.ResourceName(serviceInstance1name),
						},
					},
					Spec: smith_v1.ResourceSpec{
						Object: serviceBinding1(),
					},
				},
			},
		},
	}
	it.SetupApp(t, bundle, true, true, testServiceCatalog)
}

func testServiceCatalog(ctx context.Context, t *testing.T, cfg *it.Config, args ...interface{}) {
	cfg.AssertBundle(ctx, cfg.Bundle)
}
