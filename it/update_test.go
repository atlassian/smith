package it

import (
	"context"
	"testing"
	"time"

	"github.com/atlassian/smith"
	"github.com/atlassian/smith/examples/sleeper"
	sleeper_v1 "github.com/atlassian/smith/examples/sleeper/pkg/apis/sleeper/v1"
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"

	"github.com/ash2k/stager"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	core_v1 "k8s.io/api/core/v1"
	api_errors "k8s.io/apimachinery/pkg/api/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestUpdate(t *testing.T) {
	t.Parallel()
	cm1 := &core_v1.ConfigMap{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: "v1",
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "config2",
			Labels: map[string]string{
				"labelx": "labelxValue",
			},
		},
		Data: map[string]string{
			"a": "b",
		},
	}
	cm2 := &core_v1.ConfigMap{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: "v1",
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: cm1.Name,
			Labels: map[string]string{
				"configLabel":         "configValue",
				"overlappingLabel":    "overlappingConfigValue",
				smith.BundleNameLabel: "configLabel123",
			},
		},
		Data: map[string]string{
			"x": "y",
		},
	}
	sleeper1 := &sleeper_v1.Sleeper{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       sleeper_v1.SleeperResourceKind,
			APIVersion: sleeper_v1.SleeperResourceGroupVersion,
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "sleeper2",
			Labels: map[string]string{
				"labelx": "labelxValue",
			},
		},
		Spec: sleeper_v1.SleeperSpec{
			SleepFor:      2, // seconds,
			WakeupMessage: "Hello there!",
		},
	}
	sleeper2 := &sleeper_v1.Sleeper{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       sleeper_v1.SleeperResourceKind,
			APIVersion: sleeper_v1.SleeperResourceGroupVersion,
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: sleeper1.Name,
			Labels: map[string]string{
				"configLabel":         "configValue",
				"overlappingLabel":    "overlappingConfigValue",
				smith.BundleNameLabel: "configLabel123",
			},
		},
		Spec: sleeper_v1.SleeperSpec{
			SleepFor:      1, // seconds,
			WakeupMessage: "Hello, martians!",
		},
	}
	bundle1 := &smith_v1.Bundle{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       smith_v1.BundleResourceKind,
			APIVersion: smith_v1.BundleResourceGroupVersion,
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "bundle1",
			Labels: map[string]string{
				"bundleLabel":         "bundleValue1",
				"overlappingLabel":    "overlappingBundleValue1",
				smith.BundleNameLabel: "bundleLabel1",
			},
		},
		Spec: smith_v1.BundleSpec{
			Resources: []smith_v1.Resource{
				{
					Name: smith_v1.ResourceName(cm1.Name),
					Spec: cm1,
				},
				{
					Name: smith_v1.ResourceName(sleeper1.Name),
					Spec: sleeper1,
				},
			},
		},
	}
	bundle2 := &smith_v1.Bundle{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       smith_v1.BundleResourceKind,
			APIVersion: smith_v1.BundleResourceGroupVersion,
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "bundle1",
			Labels: map[string]string{
				"bundleLabel":         "bundleValue2",
				"overlappingLabel":    "overlappingBundleValue2",
				smith.BundleNameLabel: "bundleLabel2",
			},
		},
		Spec: smith_v1.BundleSpec{
			Resources: []smith_v1.Resource{
				{
					Name: smith_v1.ResourceName(cm2.Name),
					Spec: cm2,
				},
				{
					Name: smith_v1.ResourceName(sleeper2.Name),
					Spec: sleeper2,
				},
			},
		},
	}
	SetupApp(t, bundle1, false, true, testUpdate, cm2, sleeper1, sleeper2, bundle2)
}

func testUpdate(ctxTest context.Context, t *testing.T, cfg *Config, args ...interface{}) {
	stgr := stager.New()
	defer stgr.Shutdown()
	stage := stgr.NextStage()
	stage.StartWithContext(func(ctx context.Context) {
		apl := sleeper.App{
			RestConfig: cfg.Config,
			Namespace:  cfg.Namespace,
		}
		if e := apl.Run(ctx); e != context.Canceled && e != context.DeadlineExceeded {
			assert.NoError(t, e)
		}
	})

	cm2 := args[0].(*core_v1.ConfigMap)
	sleeper1 := args[1].(*sleeper_v1.Sleeper)
	sleeper2 := args[2].(*sleeper_v1.Sleeper)
	bundle2 := args[3].(*smith_v1.Bundle)

	cmClient := cfg.Clientset.CoreV1().ConfigMaps(cfg.Namespace)
	sClient, err := sleeper.GetSleeperClient(cfg.Config, SleeperScheme())
	require.NoError(t, err)

	ctxTimeout, cancel := context.WithTimeout(ctxTest, time.Duration(sleeper1.Spec.SleepFor+2)*time.Second)
	defer cancel()

	bundleRes1 := cfg.AssertBundle(ctxTimeout, cfg.Bundle, cfg.CreatedBundle.ResourceVersion)

	bundle2.ResourceVersion = bundleRes1.ResourceVersion
	res, err := cfg.BundleClient.SmithV1().Bundles(cfg.Namespace).Update(bundle2)
	require.NoError(t, err)

	bundleRes2 := cfg.AssertBundle(ctxTimeout, bundle2, bundle2.ResourceVersion, res.ResourceVersion)

	cfMap, err := cmClient.Get(cm2.Name, meta_v1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, map[string]string{
		"configLabel":         "configValue",
		"bundleLabel":         "bundleValue2",
		"overlappingLabel":    "overlappingConfigValue",
		smith.BundleNameLabel: cfg.Bundle.Name,
	}, cfMap.Labels)
	assert.Equal(t, cm2.Data, cfMap.Data)

	var sleeperObj sleeper_v1.Sleeper
	err = sClient.Get().
		Context(ctxTest).
		Namespace(cfg.Namespace).
		Resource(sleeper_v1.SleeperResourcePlural).
		Name(sleeper2.Name).
		Do().
		Into(&sleeperObj)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{
		"configLabel":         "configValue",
		"bundleLabel":         "bundleValue2",
		"overlappingLabel":    "overlappingConfigValue",
		smith.BundleNameLabel: cfg.Bundle.Name,
	}, sleeperObj.Labels)
	assert.Equal(t, sleeper_v1.Awake, sleeperObj.Status.State)
	assert.Equal(t, sleeper2.Spec, sleeperObj.Spec)

	emptyBundle := *cfg.Bundle
	emptyBundle.Spec.Resources = []smith_v1.Resource{}
	emptyBundle.ResourceVersion = bundleRes2.ResourceVersion
	res, err = cfg.BundleClient.SmithV1().Bundles(cfg.Namespace).Update(&emptyBundle)
	require.NoError(t, err)

	cfg.AssertBundleTimeout(ctxTest, &emptyBundle, emptyBundle.ResourceVersion, res.ResourceVersion)

	cfMap, err = cmClient.Get(cm2.Name, meta_v1.GetOptions{})
	if err == nil {
		assert.NotNil(t, cfMap.DeletionTimestamp) // Still in api but marked for deletion
	} else {
		assert.True(t, api_errors.IsNotFound(err)) // Has been removed from api already
	}
	err = sClient.Get().
		Context(ctxTest).
		Namespace(cfg.Namespace).
		Resource(sleeper_v1.SleeperResourcePlural).
		Name(sleeper2.Name).
		Do().
		Into(&sleeperObj)
	if err == nil {
		assert.NotNil(t, sleeperObj.DeletionTimestamp) // Still in api but marked for deletion
	} else {
		assert.True(t, api_errors.IsNotFound(err)) // Has been removed from api already
	}
}
