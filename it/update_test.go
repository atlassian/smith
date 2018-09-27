package it

import (
	"context"
	"testing"
	"time"

	"github.com/ash2k/stager"
	"github.com/atlassian/smith/examples/sleeper"
	sleeper_v1 "github.com/atlassian/smith/examples/sleeper/pkg/apis/sleeper/v1"
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	core_v1 "k8s.io/api/core/v1"
	api_errors "k8s.io/apimachinery/pkg/api/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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
				"configLabel":      "configValue",
				"overlappingLabel": "overlappingConfigValue",
			},
		},
		Data: map[string]string{
			"x": "y",
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
		Data: map[string][]byte{
			"a1": []byte("b1"),
			"a2": []byte("b2"),
		},
		StringData: map[string]string{
			"a2": "b3",
			"x":  "y",
		},
	}
	s2 := &core_v1.Secret{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: s1.Name,
		},
		StringData: map[string]string{
			"a1": "a2",
			"c1": "c2",
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
				"configLabel":      "configValue",
				"overlappingLabel": "overlappingConfigValue",
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
				"bundleLabel":      "bundleValue1",
				"overlappingLabel": "overlappingBundleValue1",
			},
		},
		Spec: smith_v1.BundleSpec{
			Resources: []smith_v1.Resource{
				{
					Name: smith_v1.ResourceName(cm1.Name),
					Spec: smith_v1.ResourceSpec{
						Object: cm1,
					},
				},
				{
					Name: smith_v1.ResourceName(sleeper1.Name),
					Spec: smith_v1.ResourceSpec{
						Object: sleeper1,
					},
				},
				{
					Name: smith_v1.ResourceName(s1.Name),
					Spec: smith_v1.ResourceSpec{
						Object: s1,
					},
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
				"bundleLabel":      "bundleValue2",
				"overlappingLabel": "overlappingBundleValue2",
			},
		},
		Spec: smith_v1.BundleSpec{
			Resources: []smith_v1.Resource{
				{
					Name: smith_v1.ResourceName(cm2.Name),
					Spec: smith_v1.ResourceSpec{
						Object: cm2,
					},
				},
				{
					Name: smith_v1.ResourceName(sleeper2.Name),
					Spec: smith_v1.ResourceSpec{
						Object: sleeper2,
					},
				},
				{
					Name: smith_v1.ResourceName(s2.Name),
					Spec: smith_v1.ResourceSpec{
						Object: s2,
					},
				},
			},
		},
	}
	SetupApp(t, bundle1, false, true, testUpdate, cm2, s1, s2, sleeper1, sleeper2, bundle2)
}

func testUpdate(ctxTest context.Context, t *testing.T, cfg *Config, args ...interface{}) {
	stgr := stager.New()
	defer stgr.Shutdown()
	stage := stgr.NextStage()
	stage.StartWithContext(func(ctx context.Context) {
		apl := sleeper.App{
			Logger:     cfg.Logger,
			RestConfig: cfg.Config,
			Namespace:  cfg.Namespace,
		}
		if e := apl.Run(ctx); e != context.Canceled && e != context.DeadlineExceeded {
			assert.NoError(t, e)
		}
	})

	cm2 := args[0].(*core_v1.ConfigMap)
	s1 := args[1].(*core_v1.Secret)
	s2 := args[2].(*core_v1.Secret)
	sleeper1 := args[3].(*sleeper_v1.Sleeper)
	sleeper2 := args[4].(*sleeper_v1.Sleeper)
	bundle2 := args[5].(*smith_v1.Bundle)
	convertBundleResourcesToUnstrucutred(t, bundle2)

	cmClient := cfg.MainClient.CoreV1().ConfigMaps(cfg.Namespace)
	secretClient := cfg.MainClient.CoreV1().Secrets(cfg.Namespace)
	sClient, err := sleeper.Client(cfg.Config)
	require.NoError(t, err)

	ctxTimeout, cancel := context.WithTimeout(ctxTest, time.Duration(sleeper1.Spec.SleepFor+2)*time.Second)
	defer cancel()

	bundleRes1 := cfg.AssertBundle(ctxTimeout, cfg.Bundle)

	secret, err := secretClient.Get(s1.Name, meta_v1.GetOptions{})
	require.NoError(t, err)

	assertSecretData(t, s1, secret)

	bundle2.ResourceVersion = bundleRes1.ResourceVersion
	_, err = cfg.SmithClient.SmithV1().Bundles(cfg.Namespace).Update(bundle2)
	require.NoError(t, err)

	bundleRes2 := cfg.AssertBundle(ctxTimeout, bundle2)

	cfMap, err := cmClient.Get(cm2.Name, meta_v1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, cm2.Data, cfMap.Data)
	assert.Equal(t, cm2.BinaryData, cfMap.BinaryData)

	secret, err = secretClient.Get(s2.Name, meta_v1.GetOptions{})
	require.NoError(t, err)

	assertSecretData(t, s2, secret)

	var sleeperObj sleeper_v1.Sleeper
	err = sClient.Get().
		Context(ctxTest).
		Namespace(cfg.Namespace).
		Resource(sleeper_v1.SleeperResourcePlural).
		Name(sleeper2.Name).
		Do().
		Into(&sleeperObj)
	require.NoError(t, err)
	assert.Equal(t, string(sleeper_v1.Awake), string(sleeperObj.Status.State)) // TODO workaround for https://github.com/stretchr/testify/issues/644
	assert.Equal(t, sleeper2.Spec, sleeperObj.Spec)

	emptyBundle := cfg.Bundle.DeepCopy()
	emptyBundle.Spec.Resources = []smith_v1.Resource{}
	emptyBundle.ResourceVersion = bundleRes2.ResourceVersion
	_, err = cfg.SmithClient.SmithV1().Bundles(cfg.Namespace).Update(emptyBundle)
	require.NoError(t, err)

	cfg.AssertBundleTimeout(ctxTest, emptyBundle)

	cfMap, err = cmClient.Get(cm2.Name, meta_v1.GetOptions{})
	isOrWillBeDeleted(t, cfMap, err)

	secret, err = secretClient.Get(s2.Name, meta_v1.GetOptions{})
	isOrWillBeDeleted(t, secret, err)

	err = sClient.Get().
		Context(ctxTest).
		Namespace(cfg.Namespace).
		Resource(sleeper_v1.SleeperResourcePlural).
		Name(sleeper2.Name).
		Do().
		Into(&sleeperObj)
	isOrWillBeDeleted(t, &sleeperObj, err)
}

func isOrWillBeDeleted(t *testing.T, obj runtime.Object, err error) {
	if err == nil {
		assert.NotNil(t, obj.(meta_v1.Object).GetDeletionTimestamp()) // Still in api but marked for deletion
	} else {
		assert.True(t, api_errors.IsNotFound(err)) // Has been removed from api already
	}
}

func assertSecretData(t *testing.T, expected, actual *core_v1.Secret) bool {
	expectedResultData := map[string][]byte{}
	for k, v := range expected.Data {
		expectedResultData[k] = v
	}
	for k, v := range expected.StringData {
		expectedResultData[k] = []byte(v)
	}
	return assert.Equal(t, expectedResultData, actual.Data)
}
