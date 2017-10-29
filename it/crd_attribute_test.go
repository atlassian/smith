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
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCrdAttribute(t *testing.T) {
	t.Parallel()
	sl := &sleeper_v1.Sleeper{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       sleeper_v1.SleeperResourceKind,
			APIVersion: sleeper_v1.SleeperResourceGroupVersion,
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "sleeper1",
		},
		Spec: sleeper_v1.SleeperSpec{
			SleepFor:      1, // seconds,
			WakeupMessage: "Hello, Infravators!",
		},
	}
	bundle := &smith_v1.Bundle{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       smith_v1.BundleResourceKind,
			APIVersion: smith_v1.BundleResourceGroupVersion,
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "bundle-attribute",
		},
		Spec: smith_v1.BundleSpec{
			Resources: []smith_v1.Resource{
				{
					Name: smith_v1.ResourceName(sl.Name),
					Spec: sl,
				},
			},
		},
	}
	SetupApp(t, bundle, false, true, testCrdAttribute, sl)
}

func testCrdAttribute(ctxTest context.Context, t *testing.T, cfg *Config, args ...interface{}) {
	sl := args[0].(*sleeper_v1.Sleeper)
	sClient, err := sleeper.GetSleeperClient(cfg.Config, SleeperScheme())
	require.NoError(t, err)

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

	ctxTimeout, cancel := context.WithTimeout(ctxTest, time.Duration(sl.Spec.SleepFor+3)*time.Second)
	defer cancel()

	cfg.AssertBundle(ctxTimeout, cfg.Bundle, "")

	var sleeperObj sleeper_v1.Sleeper
	require.NoError(t, sClient.Get().
		Context(ctxTest).
		Namespace(cfg.Namespace).
		Resource(sleeper_v1.SleeperResourcePlural).
		Name(sl.Name).
		Do().
		Into(&sleeperObj))

	assert.Equal(t, map[string]string{
		smith.BundleNameLabel: cfg.Bundle.Name,
	}, sleeperObj.Labels)
	assert.Equal(t, sleeper_v1.Awake, sleeperObj.Status.State)
}
