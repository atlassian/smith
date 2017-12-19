package it

import (
	"context"
	"testing"
	"time"

	"github.com/atlassian/smith/examples/sleeper"
	sleeper_v1 "github.com/atlassian/smith/examples/sleeper/pkg/apis/sleeper/v1"
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	smith_testing "github.com/atlassian/smith/pkg/util/testing"

	"github.com/ash2k/stager"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	core_v1 "k8s.io/api/core/v1"
	api_errors "k8s.io/apimachinery/pkg/api/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestAdoption(t *testing.T) {
	t.Parallel()
	cm := &core_v1.ConfigMap{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: "v1",
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "cm",
		},
		Data: map[string]string{
			"a": "b",
		},
	}
	sleeper := &sleeper_v1.Sleeper{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       sleeper_v1.SleeperResourceKind,
			APIVersion: sleeper_v1.SleeperResourceGroupVersion,
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "sleeper2",
		},
		Spec: sleeper_v1.SleeperSpec{
			SleepFor:      1, // seconds,
			WakeupMessage: "Hello there!",
		},
	}
	bundle := &smith_v1.Bundle{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       smith_v1.BundleResourceKind,
			APIVersion: smith_v1.BundleResourceGroupVersion,
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "bundle",
		},
		Spec: smith_v1.BundleSpec{
			Resources: []smith_v1.Resource{
				{
					Name: smith_v1.ResourceName(cm.Name),
					Spec: smith_v1.ResourceSpec{
						Object: cm,
					},
				},
				{
					Name: smith_v1.ResourceName(sleeper.Name),
					Spec: smith_v1.ResourceSpec{
						Object: sleeper,
					},
				},
			},
		},
	}
	SetupApp(t, bundle, false, false, testAdoption, cm, sleeper)
}

func testAdoption(ctxTest context.Context, t *testing.T, cfg *Config, args ...interface{}) {
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

	cm := args[0].(*core_v1.ConfigMap)
	sl := args[1].(*sleeper_v1.Sleeper)

	cmClient := cfg.Clientset.CoreV1().ConfigMaps(cfg.Namespace)
	sClient, err := sleeper.GetSleeperClient(cfg.Config, SleeperScheme())
	require.NoError(t, err)

	// Create orphaned ConfigMap
	cmActual, err := cmClient.Create(cm)
	require.NoError(t, err)

	// Create orphaned Sleeper
	sleeperActual := &sleeper_v1.Sleeper{}
	err = sClient.Post().
		Context(ctxTest).
		Namespace(cfg.Namespace).
		Resource(sleeper_v1.SleeperResourcePlural).
		Body(sl).
		Do().
		Into(sleeperActual)
	require.NoError(t, err)

	// Create Bundle with same resources
	bundleActual := &smith_v1.Bundle{}
	cfg.CreateObject(ctxTest, cfg.Bundle, bundleActual, smith_v1.BundleResourcePlural, cfg.BundleClient.SmithV1().RESTClient())
	cfg.CreatedBundle = bundleActual

	time.Sleep(1 * time.Second) // TODO this should be removed once race with tpr informer is fixed "no informer for tpr.atlassian.com/v1, Kind=Sleeper is registered"

	// Bundle should be in Error=true state
	bundleActual = cfg.AwaitBundleCondition(IsBundleStatusCond(cfg.Namespace, cfg.Bundle.Name, smith_v1.BundleError, smith_v1.ConditionTrue))

	smith_testing.AssertCondition(t, bundleActual, smith_v1.BundleReady, smith_v1.ConditionFalse)
	smith_testing.AssertCondition(t, bundleActual, smith_v1.BundleInProgress, smith_v1.ConditionFalse)
	cond := smith_testing.AssertCondition(t, bundleActual, smith_v1.BundleError, smith_v1.ConditionTrue)
	if cond != nil {
		assert.Equal(t, smith_v1.ResourceReasonTerminalError, cond.Reason)
		assert.Equal(t, `error processing resource(s): ["cm"]`, cond.Message)
	}

	// Point ConfigMap controller reference to Bundle
	trueVar := true
	cmActual.OwnerReferences = []meta_v1.OwnerReference{
		{
			APIVersion: smith_v1.BundleResourceGroupVersion,
			Kind:       smith_v1.BundleResourceKind,
			Name:       bundleActual.Name,
			UID:        bundleActual.UID,
			Controller: &trueVar,
		},
	}
	_, err = cmClient.Update(cmActual)
	require.NoError(t, err)

	// Bundle should be in Error=true state
	bundleActual = cfg.AwaitBundleCondition(
		IsBundleNewerCond(cfg.Namespace, cfg.Bundle.Name, bundleActual.ResourceVersion),
		IsBundleStatusCond(cfg.Namespace, cfg.Bundle.Name, smith_v1.BundleError, smith_v1.ConditionTrue))

	smith_testing.AssertCondition(t, bundleActual, smith_v1.BundleReady, smith_v1.ConditionFalse)
	smith_testing.AssertCondition(t, bundleActual, smith_v1.BundleInProgress, smith_v1.ConditionFalse)
	cond = smith_testing.AssertCondition(t, bundleActual, smith_v1.BundleError, smith_v1.ConditionTrue)
	if cond != nil {
		assert.Equal(t, smith_v1.ResourceReasonTerminalError, cond.Reason)
		assert.Equal(t, `error processing resource(s): ["sleeper2"]`, cond.Message)
	}

	// Point Sleeper controller reference to Bundle
	for { // Retry loop to handle conflicts with Sleeper controller
		sleeperActual.SetOwnerReferences([]meta_v1.OwnerReference{
			{
				APIVersion: smith_v1.BundleResourceGroupVersion,
				Kind:       smith_v1.BundleResourceKind,
				Name:       bundleActual.Name,
				UID:        bundleActual.UID,
				Controller: &trueVar,
			},
		})
		err = sClient.Put().
			Context(ctxTest).
			Namespace(cfg.Namespace).
			Resource(sleeper_v1.SleeperResourcePlural).
			Name(sleeperActual.Name).
			Body(sleeperActual).
			Do().
			Into(sleeperActual)
		if api_errors.IsConflict(err) {
			err = sClient.Get().
				Context(ctxTest).
				Namespace(cfg.Namespace).
				Resource(sleeper_v1.SleeperResourcePlural).
				Name(sleeperActual.Name).
				Do().
				Into(sleeperActual)
			require.NoError(t, err)
		} else {
			require.NoError(t, err)
			break
		}
	}

	// Bundle should reach Ready=true state
	cfg.AssertBundle(ctxTest, cfg.Bundle)

	// ConfigMap should have BlockOwnerDeletion updated
	cmActual, err = cmClient.Get(cm.Name, meta_v1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, []meta_v1.OwnerReference{
		{
			APIVersion:         smith_v1.BundleResourceGroupVersion,
			Kind:               smith_v1.BundleResourceKind,
			Name:               bundleActual.Name,
			UID:                bundleActual.UID,
			Controller:         &trueVar,
			BlockOwnerDeletion: &trueVar, // Check that this is set to true
		},
	}, cmActual.GetOwnerReferences())

	// Sleeper should have BlockOwnerDeletion updated
	err = sClient.Get().
		Context(ctxTest).
		Namespace(cfg.Namespace).
		Resource(sleeper_v1.SleeperResourcePlural).
		Name(sleeperActual.Name).
		Do().
		Into(sleeperActual)
	require.NoError(t, err)
	assert.Equal(t, []meta_v1.OwnerReference{
		{
			APIVersion:         smith_v1.BundleResourceGroupVersion,
			Kind:               smith_v1.BundleResourceKind,
			Name:               bundleActual.Name,
			UID:                bundleActual.UID,
			Controller:         &trueVar,
			BlockOwnerDeletion: &trueVar, // Check that this is set to true
		},
	}, sleeperActual.GetOwnerReferences())
}
