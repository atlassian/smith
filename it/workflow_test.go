package it

import (
	"context"
	"testing"

	"github.com/atlassian/smith"
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	core_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestWorkflow(t *testing.T) {
	t.Parallel()
	c1 := &core_v1.ConfigMap{
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
	s1 := &core_v1.Secret{
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
	bundle := &smith_v1.Bundle{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       smith_v1.BundleResourceKind,
			APIVersion: smith_v1.BundleResourceGroupVersion,
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "bundle1",
			Labels: map[string]string{
				"bundleLabel":         "bundleValue",
				"overlappingLabel":    "overlappingBundleValue",
				smith.BundleNameLabel: "bundleLabel123",
			},
		},
		Spec: smith_v1.BundleSpec{
			Resources: []smith_v1.Resource{
				{
					Name:      "config1res",
					DependsOn: []smith_v1.ResourceName{"secret2res"},
					Spec:      c1,
				},
				{
					Name: "secret2res",
					Spec: s1,
				},
			},
		},
	}
	SetupApp(t, bundle, false, true, testWorkflow)
}

func testWorkflow(ctx context.Context, t *testing.T, cfg *Config, args ...interface{}) {
	bundleRes := cfg.AssertBundleTimeout(ctx, cfg.Bundle, cfg.CreatedBundle.ResourceVersion)

	cfMap, err := cfg.Clientset.CoreV1().ConfigMaps(cfg.Namespace).Get("config1", meta_v1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, map[string]string{
		"configLabel":         "configValue",
		"bundleLabel":         "bundleValue",
		"overlappingLabel":    "overlappingConfigValue",
		smith.BundleNameLabel: bundleRes.Name,
	}, cfMap.GetLabels())

	secret, err := cfg.Clientset.CoreV1().Secrets(cfg.Namespace).Get("secret1", meta_v1.GetOptions{})
	require.NoError(t, err)
	trueRef := true
	assert.Equal(t, []meta_v1.OwnerReference{
		{
			APIVersion:         smith_v1.BundleResourceGroupVersion,
			Kind:               smith_v1.BundleResourceKind,
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
	assert.Equal(t, []meta_v1.OwnerReference{
		{
			APIVersion:         smith_v1.BundleResourceGroupVersion,
			Kind:               smith_v1.BundleResourceKind,
			Name:               bundleRes.Name,
			UID:                bundleRes.UID,
			Controller:         &trueRef,
			BlockOwnerDeletion: &trueRef,
		},
	}, secret.GetOwnerReferences())
}
