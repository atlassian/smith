// +build integration

package integration_tests

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/atlassian/smith"
	"github.com/atlassian/smith/examples/tprattribute"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestTprAttribute(t *testing.T) {
	sleeper := &tprattribute.Sleeper{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       tprattribute.SleeperResourceKind,
			APIVersion: tprattribute.SleeperResourceGroupVersion,
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "sleeper1",
		},
		Spec: tprattribute.SleeperSpec{
			SleepFor:      1, // seconds,
			WakeupMessage: "Hello, Infravators!",
		},
	}
	bundle := &smith.Bundle{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       smith.BundleResourceKind,
			APIVersion: smith.BundleResourceGroupVersion,
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "bundle-attribute",
		},
		Spec: smith.BundleSpec{
			Resources: []smith.Resource{
				{
					Name: smith.ResourceName(sleeper.Name),
					Spec: sleeper,
				},
			},
		},
	}
	setupApp(t, bundle, false, true, testTprAttribute, sleeper)
}

func testTprAttribute(t *testing.T, ctx context.Context, cfg *itConfig, args ...interface{}) {

	sleeper := args[0].(*tprattribute.Sleeper)
	sClient, err := tprattribute.GetSleeperTprClient(cfg.config, sleeperScheme())
	require.NoError(t, err)

	var wg sync.WaitGroup
	defer wg.Wait()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	wg.Add(1)
	go func() {
		defer wg.Done()
		apl := tprattribute.App{
			RestConfig: cfg.config,
		}
		if e := apl.Run(ctx); e != context.Canceled && e != context.DeadlineExceeded {
			assert.NoError(t, e)
		}
	}()

	ctxTimeout, cancel := context.WithTimeout(ctx, time.Duration(sleeper.Spec.SleepFor+3)*time.Second)
	defer cancel()

	assertBundle(t, ctxTimeout, cfg.store, cfg.namespace, cfg.bundle, "")

	var sleeperObj tprattribute.Sleeper
	require.NoError(t, sClient.Get().
		Namespace(cfg.namespace).
		Resource(tprattribute.SleeperResourcePath).
		Name(sleeper.Name).
		Do().
		Into(&sleeperObj))

	assert.Equal(t, map[string]string{
		smith.BundleNameLabel: cfg.bundle.Name,
	}, sleeperObj.Labels)
	assert.Equal(t, tprattribute.Awake, sleeperObj.Status.State)
}
