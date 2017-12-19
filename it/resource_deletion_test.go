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
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPreExistingResourceDeletion(t *testing.T) {
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
	sl := &sleeper_v1.Sleeper{
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
					Name: smith_v1.ResourceName(sl.Name),
					Spec: smith_v1.ResourceSpec{
						Object: sl,
					},
				},
			},
		},
	}
	SetupApp(t, bundle, false, false, testResourceDeletion, cm, sl)
}

func testResourceDeletion(ctxTest context.Context, t *testing.T, cfg *Config, args ...interface{}) {
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
	resCm := smith_v1.ResourceName(cm.Name)
	sl := args[1].(*sleeper_v1.Sleeper)
	resSl := smith_v1.ResourceName(sl.Name)

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
	smith_testing.AssertResourceCondition(cfg.T, bundleActual, resCm, smith_v1.ResourceBlocked, smith_v1.ConditionFalse)
	smith_testing.AssertResourceCondition(cfg.T, bundleActual, resCm, smith_v1.ResourceInProgress, smith_v1.ConditionFalse)
	smith_testing.AssertResourceCondition(cfg.T, bundleActual, resCm, smith_v1.ResourceReady, smith_v1.ConditionFalse)
	resCond := smith_testing.AssertResourceCondition(cfg.T, bundleActual, resCm, smith_v1.ResourceError, smith_v1.ConditionTrue)
	if resCond != nil {
		assert.Equal(cfg.T, "object is not owned by the Bundle", resCond.Message)
	}

	resCond = smith_testing.AssertResourceCondition(cfg.T, bundleActual, resSl, smith_v1.ResourceBlocked, smith_v1.ConditionTrue)
	if resCond != nil {
		assert.Equal(cfg.T, "Some other resource is in error state", resCond.Message)
		assert.Equal(cfg.T, smith_v1.ResourceReasonOtherResourceError, resCond.Reason)
	}
	smith_testing.AssertResourceCondition(cfg.T, bundleActual, resSl, smith_v1.ResourceInProgress, smith_v1.ConditionFalse)
	smith_testing.AssertResourceCondition(cfg.T, bundleActual, resSl, smith_v1.ResourceReady, smith_v1.ConditionFalse)
	smith_testing.AssertResourceCondition(cfg.T, bundleActual, resSl, smith_v1.ResourceError, smith_v1.ConditionFalse)

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

	// Delete conflicting Sleeper
	err = sClient.Delete().
		Context(ctxTest).
		Namespace(cfg.Namespace).
		Resource(sleeper_v1.SleeperResourcePlural).
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
	_, err = cmClient.Get(cm.Name, meta_v1.GetOptions{})
	require.NoError(t, err)

	// Sleeper should have BlockOwnerDeletion updated
	err = sClient.Get().
		Namespace(cfg.Namespace).
		Resource(sleeper_v1.SleeperResourcePlural).
		Name(sleeperActual.Name).
		Do().
		Into(sleeperActual)
	require.NoError(t, err)
}
