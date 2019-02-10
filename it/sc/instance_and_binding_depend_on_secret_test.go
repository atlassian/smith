package sc

import (
	"context"
	"testing"

	"github.com/atlassian/smith/it"
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	scClientset "github.com/kubernetes-incubator/service-catalog/pkg/client/clientset_generated/clientset"
	"github.com/stretchr/testify/require"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/tools/cache"
	toolswatch "k8s.io/client-go/tools/watch"
)

// Given: a Bundle, containing ServiceInstance and/or ServiceBinding.
// Given: that ServiceInstance and/or ServiceBinding have parametersFrom block using a Secret
// When: that Secret is created/updated/deleted
// Then: Bundle should be enqueued for processing
func TestSecretToBindingAndInstanceToBundleIndex(t *testing.T) {
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
						Object: serviceInstance1withParametersFrom(),
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
						Object: serviceBinding1withParametersFrom(),
					},
				},
			},
		},
	}
	it.SetupApp(t, bundle, true, false, testSecretToBindingAndInstanceToBundleIndex)
}

func testSecretToBindingAndInstanceToBundleIndex(ctx context.Context, t *testing.T, cfg *it.Config, args ...interface{}) {
	scClient, err := scClientset.NewForConfig(cfg.Config)
	require.NoError(t, err)

	// initial create of Secret objects
	s1, err := cfg.MainClient.CoreV1().Secrets(cfg.Namespace).Create(secret1())
	require.NoError(t, err)
	s2, err := cfg.MainClient.CoreV1().Secrets(cfg.Namespace).Create(secret2())
	require.NoError(t, err)

	res := &smith_v1.Bundle{}
	cfg.CreateObject(ctx, cfg.Bundle, res, smith_v1.BundleResourcePlural, cfg.SmithClient.SmithV1().RESTClient())
	cfg.CreatedBundle = res

	// Wait for stable state
	cfg.AssertBundle(ctx, cfg.Bundle)

	si, err := scClient.ServicecatalogV1beta1().ServiceInstances(cfg.Namespace).Get(serviceInstance1name, meta_v1.GetOptions{})
	require.NoError(t, err)

	// Update Secret1
	s1.Data[secret1credentialsKey] = []byte(`{"token": "NEW token" }`)
	_, err = cfg.MainClient.CoreV1().Secrets(cfg.Namespace).Update(s1)
	require.NoError(t, err)

	// Wait for updated UpdateRequests
	lw := cache.NewListWatchFromClient(scClient.ServicecatalogV1beta1().RESTClient(), "serviceinstances", cfg.Namespace, fields.Everything())
	cond := it.IsServiceInstanceUpdateRequestsCond(t, cfg.Namespace, si.Name, si.Spec.UpdateRequests)
	_, err = toolswatch.UntilWithSync(ctx, lw, &sc_v1b1.ServiceInstance{}, nil, cond)
	require.NoError(t, err)

	// Update Secret2
	s2.Data[secret1credentialsKey] = []byte(`{"token": "NEW token" }`)
	_, err = cfg.MainClient.CoreV1().Secrets(cfg.Namespace).Update(s2)
	require.NoError(t, err)

	// Nothing to wait for, ServiceBinding does not have a way to force an update.
}
