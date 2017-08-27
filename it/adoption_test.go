package it

import (
	"context"
	"testing"
	"time"

	"github.com/atlassian/smith/examples/sleeper"
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"

	"github.com/ash2k/stager"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	api_errors "k8s.io/apimachinery/pkg/api/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	api_v1 "k8s.io/client-go/pkg/api/v1"
)

func TestAdoption(t *testing.T) {
	t.Parallel()
	cm := &api_v1.ConfigMap{
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
	sleeper := &sleeper.Sleeper{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       sleeper.SleeperResourceKind,
			APIVersion: sleeper.SleeperResourceGroupVersion,
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "sleeper2",
		},
		Spec: sleeper.SleeperSpec{
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
					Spec: cm,
				},
				{
					Name: smith_v1.ResourceName(sleeper.Name),
					Spec: sleeper,
				},
			},
		},
	}
	SetupApp(t, bundle, false, false, testAdoption, cm, sleeper)
}

func testAdoption(t *testing.T, ctxTest context.Context, cfg *ItConfig, args ...interface{}) {
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

	cm := args[0].(*api_v1.ConfigMap)
	sl := args[1].(*sleeper.Sleeper)

	cmClient := cfg.Clientset.CoreV1().ConfigMaps(cfg.Namespace)
	sClient, err := sleeper.GetSleeperClient(cfg.Config, SleeperScheme())
	require.NoError(t, err)

	// Create orphaned ConfigMap
	cmActual, err := cmClient.Create(cm)
	require.NoError(t, err)

	// Create orphaned Sleeper
	sleeperActual := &sleeper.Sleeper{}
	err = sClient.Post().
		Context(ctxTest).
		Namespace(cfg.Namespace).
		Resource(sleeper.SleeperResourcePlural).
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

	AssertCondition(t, bundleActual, smith_v1.BundleReady, smith_v1.ConditionFalse)
	AssertCondition(t, bundleActual, smith_v1.BundleInProgress, smith_v1.ConditionFalse)
	cond := AssertCondition(t, bundleActual, smith_v1.BundleError, smith_v1.ConditionTrue)
	if cond != nil {
		assert.Equal(t, "TerminalError", cond.Reason)
		assert.Equal(t, "object /v1, Kind=ConfigMap \"cm\" is not owned by the Bundle", cond.Message)
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
	cmActual, err = cmClient.Update(cmActual)
	require.NoError(t, err)

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
			Resource(sleeper.SleeperResourcePlural).
			Name(sleeperActual.Name).
			Body(sleeperActual).
			Do().
			Into(sleeperActual)
		if api_errors.IsConflict(err) {
			err = sClient.Get().
				Context(ctxTest).
				Namespace(cfg.Namespace).
				Resource(sleeper.SleeperResourcePlural).
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
		Resource(sleeper.SleeperResourcePlural).
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
