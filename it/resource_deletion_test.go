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
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	api_v1 "k8s.io/client-go/pkg/api/v1"
)

func TestResourceDeletion(t *testing.T) {
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
	sl := &sleeper.Sleeper{
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
					Name: smith_v1.ResourceName(sl.Name),
					Spec: sl,
				},
			},
		},
	}
	SetupApp(t, bundle, false, false, testResourceDeletion, cm, sl)
}

func testResourceDeletion(t *testing.T, ctxTest context.Context, cfg *ItConfig, args ...interface{}) {
	stgr := stager.New()
	defer stgr.Shutdown()
	stage := stgr.NextStage()
	stage.StartWithContext(func(ctx context.Context) {
		apl := sleeper.App{
			RestConfig: cfg.Config,
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
	cfg.CleanupLater(cmActual)

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
	cfg.CleanupLater(sleeperActual)

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

	// Delete conflicting ConfigMap
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
	err = cmClient.Delete(cmActual.Name, &meta_v1.DeleteOptions{
		Preconditions: &meta_v1.Preconditions{
			UID: &cmActual.UID,
		},
	})
	require.NoError(t, err)

	err = sClient.Delete().
		Context(ctxTest).
		Namespace(cfg.Namespace).
		Resource(sleeper.SleeperResourcePlural).
		Name(sleeperActual.Name).
		Body(&meta_v1.DeleteOptions{
			Preconditions: &meta_v1.Preconditions{
				UID: &sleeperActual.UID,
			},
		}).
		Do().
		Error()
	require.NoError(t, err)

	// Bundle should reach Ready=true state
	cfg.AssertBundle(ctxTest, cfg.Bundle)

	// ConfigMap should exist by now
	cmActual, err = cmClient.Get(cm.Name, meta_v1.GetOptions{})
	require.NoError(t, err)

	// Sleeper should have BlockOwnerDeletion updated
	err = sClient.Get().
		Namespace(cfg.Namespace).
		Resource(sleeper.SleeperResourcePlural).
		Name(sleeperActual.Name).
		Do().
		Into(sleeperActual)
	require.NoError(t, err)
}
