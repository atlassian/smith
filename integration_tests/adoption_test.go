// +build integration

package integration_tests

import (
	"context"
	"testing"
	"time"

	"github.com/atlassian/smith"
	"github.com/atlassian/smith/examples/tprattribute"
	"github.com/atlassian/smith/pkg/util/wait"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	api_v1 "k8s.io/client-go/pkg/api/v1"
)

func TestAdoption(t *testing.T) {
	// TODO uncomment when https://github.com/kubernetes/kubernetes/issues/46817 is fixed
	t.SkipNow()
	cm := &api_v1.ConfigMap{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: "v1",
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "cm",
			Labels: map[string]string{
				smith.BundleNameLabel: "bundle",
			},
		},
		Data: map[string]string{
			"a": "b",
		},
	}
	sleeper := &tprattribute.Sleeper{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       tprattribute.SleeperResourceKind,
			APIVersion: tprattribute.SleeperResourceGroupVersion,
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "sleeper2",
			Labels: map[string]string{
				smith.BundleNameLabel: "bundle",
			},
		},
		Spec: tprattribute.SleeperSpec{
			SleepFor:      1, // seconds,
			WakeupMessage: "Hello there!",
		},
	}
	bundle := &smith.Bundle{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       smith.BundleResourceKind,
			APIVersion: smith.BundleResourceGroupVersion,
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "bundle",
		},
		Spec: smith.BundleSpec{
			Resources: []smith.Resource{
				{
					Name: smith.ResourceName(cm.Name),
					Spec: cm,
				},
				{
					Name: smith.ResourceName(sleeper.Name),
					Spec: sleeper,
				},
			},
		},
	}
	setupApp(t, bundle, false, false, testAdoption, cm, sleeper)
}

func testAdoption(t *testing.T, ctx context.Context, cfg *itConfig, args ...interface{}) {
	var wg wait.Group
	defer wg.Wait()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	wg.Start(func() {
		apl := tprattribute.App{
			RestConfig: cfg.config,
		}
		if e := apl.Run(ctx); e != context.Canceled && e != context.DeadlineExceeded {
			assert.NoError(t, e)
		}
	})

	cm := args[0].(*api_v1.ConfigMap)
	sleeper := args[1].(*tprattribute.Sleeper)

	cmClient := cfg.clientset.CoreV1().ConfigMaps(cfg.namespace)
	sClient, err := tprattribute.GetSleeperTprClient(cfg.config, sleeperScheme())
	require.NoError(t, err)

	// Create orphaned ConfigMap
	cmActual, err := cmClient.Create(cm)
	require.NoError(t, err)

	// Create orphaned Sleeper
	sleeperActual := &tprattribute.Sleeper{}
	err = sClient.Post().
		Namespace(cfg.namespace).
		Resource(tprattribute.SleeperResourcePath).
		Body(sleeper).
		Do().
		Into(sleeperActual)
	require.NoError(t, err)

	// Create Bundle with same resources
	bundleActual := &smith.Bundle{}
	createObject(t, cfg.bundle, bundleActual, cfg.namespace, smith.BundleResourcePath, cfg.bundleClient)
	cfg.createdBundle = bundleActual

	time.Sleep(1 * time.Second) // TODO this should be removed once race with tpr informer is fixed "no informer for tpr.atlassian.com/v1, Kind=Sleeper is registered"

	// Bundle should be in Error=true state
	obj, err := cfg.store.AwaitObjectCondition(ctx, smith.BundleGVK, cfg.namespace, cfg.bundle.Name, isBundleError)
	require.NoError(t, err)
	bundleActual = obj.(*smith.Bundle)

	assertCondition(t, bundleActual, smith.BundleReady, smith.ConditionFalse)
	assertCondition(t, bundleActual, smith.BundleInProgress, smith.ConditionFalse)
	cond := assertCondition(t, bundleActual, smith.BundleError, smith.ConditionTrue)
	if cond != nil {
		assert.Equal(t, "TerminalError", cond.Reason)
		assert.Equal(t, "object /v1, Kind=ConfigMap \"cm\" is not owned by the Bundle", cond.Message)
	}

	// Point ConfigMap controller reference to Bundle
	trueVar := true
	refs := []meta_v1.OwnerReference{
		{
			APIVersion: smith.BundleResourceGroupVersion,
			Kind:       smith.BundleResourceKind,
			Name:       bundleActual.Name,
			UID:        bundleActual.UID,
			Controller: &trueVar,
		},
	}
	cmActual.OwnerReferences = refs
	cmActual, err = cmClient.Update(cmActual)
	require.NoError(t, err)

	// Point Sleeper controller reference to Bundle
	for { // Retry loop to handle conflicts with Sleeper controller
		sleeperActual.SetOwnerReferences(refs)
		err = sClient.Put().
			Namespace(cfg.namespace).
			Resource(tprattribute.SleeperResourcePath).
			Name(sleeperActual.Name).
			Body(sleeperActual).
			Do().
			Into(sleeperActual)
		if kerrors.IsConflict(err) {
			err = sClient.Get().
				Namespace(cfg.namespace).
				Resource(tprattribute.SleeperResourcePath).
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
	assertBundle(t, ctx, cfg.store, cfg.namespace, cfg.bundle)

	// ConfigMap should have BlockOwnerDeletion updated
	cmActual, err = cmClient.Get(cm.Name, meta_v1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, []meta_v1.OwnerReference{
		{
			APIVersion:         smith.BundleResourceGroupVersion,
			Kind:               smith.BundleResourceKind,
			Name:               bundleActual.Name,
			UID:                bundleActual.UID,
			Controller:         &trueVar,
			BlockOwnerDeletion: &trueVar, // Check that this is set to true
		},
	}, cmActual.GetOwnerReferences())

	// Sleeper should have BlockOwnerDeletion updated
	err = sClient.Get().
		Namespace(cfg.namespace).
		Resource(tprattribute.SleeperResourcePath).
		Name(sleeperActual.Name).
		Do().
		Into(sleeperActual)
	require.NoError(t, err)
	assert.Equal(t, []meta_v1.OwnerReference{
		{
			APIVersion:         smith.BundleResourceGroupVersion,
			Kind:               smith.BundleResourceKind,
			Name:               bundleActual.Name,
			UID:                bundleActual.UID,
			Controller:         &trueVar,
			BlockOwnerDeletion: &trueVar, // Check that this is set to true
		},
	}, sleeperActual.GetOwnerReferences())
}
