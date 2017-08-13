// +build integration

package integration_tests

import (
	"context"
	"testing"
	"time"

	"github.com/atlassian/smith"
	"github.com/atlassian/smith/examples/sleeper"
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"

	"github.com/ash2k/stager"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	api_errors "k8s.io/apimachinery/pkg/api/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	api_v1 "k8s.io/client-go/pkg/api/v1"
)

func TestUpdate(t *testing.T) {
	cm1 := &api_v1.ConfigMap{
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
	cm2 := &api_v1.ConfigMap{
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
	sleeper1 := &sleeper.Sleeper{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       sleeper.SleeperResourceKind,
			APIVersion: sleeper.SleeperResourceGroupVersion,
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "sleeper2",
			Labels: map[string]string{
				"labelx": "labelxValue",
			},
		},
		Spec: sleeper.SleeperSpec{
			SleepFor:      2, // seconds,
			WakeupMessage: "Hello there!",
		},
	}
	sleeper2 := &sleeper.Sleeper{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       sleeper.SleeperResourceKind,
			APIVersion: sleeper.SleeperResourceGroupVersion,
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: sleeper1.Name,
			Labels: map[string]string{
				"configLabel":         "configValue",
				"overlappingLabel":    "overlappingConfigValue",
				smith.BundleNameLabel: "configLabel123",
			},
		},
		Spec: sleeper.SleeperSpec{
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
	setupApp(t, bundle1, false, true, testUpdate, cm2, sleeper1, sleeper2, bundle2)
}

func testUpdate(t *testing.T, ctxTest context.Context, cfg *itConfig, args ...interface{}) {
	stgr := stager.New()
	defer stgr.Shutdown()
	stage := stgr.NextStage()
	stage.StartWithContext(func(ctx context.Context) {
		apl := sleeper.App{
			RestConfig: cfg.config,
		}
		if e := apl.Run(ctx); e != context.Canceled && e != context.DeadlineExceeded {
			assert.NoError(t, e)
		}
	})

	cm2 := args[0].(*api_v1.ConfigMap)
	sleeper1 := args[1].(*sleeper.Sleeper)
	sleeper2 := args[2].(*sleeper.Sleeper)
	bundle2 := args[3].(*smith_v1.Bundle)

	cmClient := cfg.clientset.CoreV1().ConfigMaps(cfg.namespace)
	sClient, err := sleeper.GetSleeperClient(cfg.config, sleeperScheme())
	require.NoError(t, err)

	ctxTimeout, cancel := context.WithTimeout(ctxTest, time.Duration(sleeper1.Spec.SleepFor+2)*time.Second)
	defer cancel()

	bundleRes1 := cfg.assertBundle(ctxTimeout, cfg.bundle, cfg.createdBundle.ResourceVersion)

	res := &smith_v1.Bundle{}
	bundle2.ResourceVersion = bundleRes1.ResourceVersion
	require.NoError(t, cfg.bundleClient.Put().
		Context(ctxTest).
		Namespace(cfg.namespace).
		Resource(smith_v1.BundleResourcePlural).
		Name(bundle2.Name).
		Body(bundle2).
		Do().
		Into(res))

	bundleRes2 := cfg.assertBundle(ctxTimeout, bundle2, bundle2.ResourceVersion, res.ResourceVersion)

	cfMap, err := cmClient.Get(cm2.Name, meta_v1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, map[string]string{
		"configLabel":         "configValue",
		"bundleLabel":         "bundleValue2",
		"overlappingLabel":    "overlappingConfigValue",
		smith.BundleNameLabel: cfg.bundle.Name,
	}, cfMap.Labels)
	assert.Equal(t, cm2.Data, cfMap.Data)

	var sleeperObj sleeper.Sleeper
	err = sClient.Get().
		Context(ctxTest).
		Namespace(cfg.namespace).
		Resource(sleeper.SleeperResourcePlural).
		Name(sleeper2.Name).
		Do().
		Into(&sleeperObj)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{
		"configLabel":         "configValue",
		"bundleLabel":         "bundleValue2",
		"overlappingLabel":    "overlappingConfigValue",
		smith.BundleNameLabel: cfg.bundle.Name,
	}, sleeperObj.Labels)
	assert.Equal(t, sleeper.Awake, sleeperObj.Status.State)
	assert.Equal(t, sleeper2.Spec, sleeperObj.Spec)

	emptyBundle := *cfg.bundle
	emptyBundle.Spec.Resources = []smith_v1.Resource{}
	emptyBundle.ResourceVersion = bundleRes2.ResourceVersion
	require.NoError(t, cfg.bundleClient.Put().
		Context(ctxTest).
		Namespace(cfg.namespace).
		Resource(smith_v1.BundleResourcePlural).
		Name(emptyBundle.Name).
		Body(&emptyBundle).
		Do().
		Into(res))

	cfg.assertBundleTimeout(ctxTest, &emptyBundle, emptyBundle.ResourceVersion, res.ResourceVersion)

	cfMap, err = cmClient.Get(cm2.Name, meta_v1.GetOptions{})
	if err == nil {
		assert.NotNil(t, cfMap.DeletionTimestamp) // Still in api but marked for deletion
	} else {
		assert.True(t, api_errors.IsNotFound(err)) // Has been removed from api already
	}
	err = sClient.Get().
		Context(ctxTest).
		Namespace(cfg.namespace).
		Resource(sleeper.SleeperResourcePlural).
		Name(sleeper2.Name).
		Do().
		Into(&sleeperObj)
	if err == nil {
		assert.NotNil(t, sleeperObj.DeletionTimestamp) // Still in api but marked for deletion
	} else {
		assert.True(t, api_errors.IsNotFound(err)) // Has been removed from api already
	}
}
