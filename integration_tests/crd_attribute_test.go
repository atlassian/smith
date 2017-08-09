// +build integration

package integration_tests

import (
	"context"
	"testing"
	"time"

	"github.com/atlassian/smith"
	"github.com/atlassian/smith/examples/sleeper"

	"github.com/ash2k/stager"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCrdAttribute(t *testing.T) {
	sl := &sleeper.Sleeper{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       sleeper.SleeperResourceKind,
			APIVersion: sleeper.SleeperResourceGroupVersion,
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "sleeper1",
		},
		Spec: sleeper.SleeperSpec{
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
					Name: smith.ResourceName(sl.Name),
					Spec: sl,
				},
			},
		},
	}
	setupApp(t, bundle, false, true, testCrdAttribute, sl)
}

func testCrdAttribute(t *testing.T, ctxTest context.Context, cfg *itConfig, args ...interface{}) {
	sl := args[0].(*sleeper.Sleeper)
	sClient, err := sleeper.GetSleeperClient(cfg.config, sleeperScheme())
	require.NoError(t, err)

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

	ctxTimeout, cancel := context.WithTimeout(ctxTest, time.Duration(sl.Spec.SleepFor+3)*time.Second)
	defer cancel()

	cfg.assertBundle(ctxTimeout, cfg.bundle, "")

	var sleeperObj sleeper.Sleeper
	require.NoError(t, sClient.Get().
		Context(ctxTest).
		Namespace(cfg.namespace).
		Resource(sleeper.SleeperResourcePlural).
		Name(sl.Name).
		Do().
		Into(&sleeperObj))

	assert.Equal(t, map[string]string{
		smith.BundleNameLabel: cfg.bundle.Name,
	}, sleeperObj.Labels)
	assert.Equal(t, sleeper.Awake, sleeperObj.Status.State)
}
