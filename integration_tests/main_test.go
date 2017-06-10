// +build integration

package integration_tests

import (
	"context"
	"testing"

	"github.com/atlassian/smith"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	api_v1 "k8s.io/client-go/pkg/api/v1"
)

func TestWorkflow(t *testing.T) {
	c1 := &api_v1.ConfigMap{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: "v1",
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "config1",
			Labels: map[string]string{
				"configLabel":         "configValue",
				"overlappingLabel":    "overlappingConfigValue",
				smith.BundleNameLabel: "configLabel123",
			},
		},
		Data: map[string]string{
			"a": "b",
		},
	}
	s1 := &api_v1.Secret{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "secret1",
		},
		StringData: map[string]string{
			"a": "b",
		},
	}
	bundle := &smith.Bundle{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       smith.BundleResourceKind,
			APIVersion: smith.BundleResourceGroupVersion,
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "bundle1",
			Labels: map[string]string{
				"bundleLabel":         "bundleValue",
				"overlappingLabel":    "overlappingBundleValue",
				smith.BundleNameLabel: "bundleLabel123",
			},
		},
		Spec: smith.BundleSpec{
			Resources: []smith.Resource{
				{
					Name:      "config1res",
					DependsOn: []smith.ResourceName{"secret2res"},
					Spec:      c1,
				},
				{
					Name: "secret2res",
					Spec: s1,
				},
			},
		},
	}
	setupApp(t, bundle, false, true, testWorkflow)
}

func testWorkflow(t *testing.T, ctx context.Context, cfg *itConfig, args ...interface{}) {
	bundleRes := assertBundleTimeout(t, ctx, cfg.store, cfg.namespace, cfg.bundle, cfg.createdBundle.ResourceVersion)

	cfMap, err := cfg.clientset.CoreV1().ConfigMaps(cfg.namespace).Get("config1", meta_v1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, map[string]string{
		"configLabel":         "configValue",
		"bundleLabel":         "bundleValue",
		"overlappingLabel":    "overlappingConfigValue",
		smith.BundleNameLabel: bundleRes.Name,
	}, cfMap.GetLabels())

	secret, err := cfg.clientset.CoreV1().Secrets(cfg.namespace).Get("secret1", meta_v1.GetOptions{})
	require.NoError(t, err)
	trueRef := true
	assert.Equal(t, []meta_v1.OwnerReference{
		{
			APIVersion:         smith.BundleResourceGroupVersion,
			Kind:               smith.BundleResourceKind,
			Name:               bundleRes.Name,
			UID:                bundleRes.UID,
			Controller:         &trueRef,
			BlockOwnerDeletion: &trueRef,
		},
		{
			APIVersion:         "v1",
			Kind:               "Secret",
			Name:               secret.Name,
			UID:                secret.UID,
			BlockOwnerDeletion: &trueRef,
		},
	}, cfMap.GetOwnerReferences())
	// TODO uncomment when https://github.com/kubernetes/kubernetes/issues/46817 is fixed
	//assert.Equal(t, []meta_v1.OwnerReference{
	//	{
	//		APIVersion:         smith.BundleResourceGroupVersion,
	//		Kind:               smith.BundleResourceKind,
	//		Name:               bundleRes.Name,
	//		UID:                bundleRes.UID,
	//		Controller:         &trueRef,
	//		BlockOwnerDeletion: &trueRef,
	//	},
	//}, secret.GetOwnerReferences())
}
