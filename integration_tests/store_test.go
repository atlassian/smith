// +build integration

package integration_tests

import (
	"context"
	"testing"
	"time"

	"github.com/atlassian/smith/pkg/client"
	"github.com/atlassian/smith/pkg/store"
	"github.com/atlassian/smith/pkg/util/wait"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	api_errors "k8s.io/apimachinery/pkg/api/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/informers"
	api_v1 "k8s.io/client-go/pkg/api/v1"
)

func TestMultiStore(t *testing.T) {
	_, clientset, _ := testSetup(t)

	multiStore := store.NewMulti(client.BundleScheme().DeepCopy)

	var wgStore wait.Group
	defer wgStore.Wait() // await multiStore termination

	ctxStore, cancelStore := context.WithCancel(context.Background())
	defer cancelStore() // signal multiStore to stop
	wgStore.StartWithContext(ctxStore, multiStore.Run)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	configMapGvk := api_v1.SchemeGroupVersion.WithKind("ConfigMap")

	informerFactory := informers.NewSharedInformerFactory(clientset, 1*time.Minute)
	configMapInf := informerFactory.Core().V1().ConfigMaps().Informer()
	multiStore.AddInformer(configMapGvk, configMapInf)
	informerFactory.Start(ctx.Done()) // Must be after multiStore.AddInformer()

	t.Run("timeout", func(t *testing.T) {
		ctxTimeout, cancelTimeout1 := context.WithTimeout(ctx, 1*time.Second)
		defer cancelTimeout1()

		_, err := multiStore.AwaitObject(ctxTimeout, configMapGvk, useNamespace, "i-do-not-exist-13123123123")
		assert.EqualError(t, err, context.DeadlineExceeded.Error())
	})
	t.Run("create", func(t *testing.T) {
		mapName := "i-do-not-exist-yet"
		cm := &api_v1.ConfigMap{
			ObjectMeta: meta_v1.ObjectMeta{
				Name: mapName,
			},
			Data: map[string]string{
				"a": "b",
			},
		}

		err := clientset.CoreV1().ConfigMaps(useNamespace).Delete(mapName, nil)
		if err != nil && !api_errors.IsNotFound(err) {
			require.NoError(t, err)
		}

		ctxTimeout, cancelTimeout2 := context.WithTimeout(ctx, 5*time.Second)
		defer cancelTimeout2()

		var obj runtime.Object
		var wg wait.Group
		wg.Start(func() {
			obj, err = multiStore.AwaitObject(ctxTimeout, configMapGvk, useNamespace, mapName)
		})
		time.Sleep(1 * time.Second)
		cm, errCreate := clientset.CoreV1().ConfigMaps(useNamespace).Create(cm)
		require.NoError(t, errCreate)
		defer func() {
			// Cleanup after successful create
			err = clientset.CoreV1().ConfigMaps(useNamespace).Delete(mapName, nil)
			assert.NoError(t, err)
		}()
		wg.Wait()
		require.NoError(t, err)
		cm.GetObjectKind().SetGroupVersionKind(api_v1.SchemeGroupVersion.WithKind("ConfigMap"))
		assert.Equal(t, cm, obj)
	})
	t.Run("remove informer", func(t *testing.T) {
		var err error
		mapName := "i-do-not-exist-234234"
		ctxTimeout, cancelTimeout := context.WithTimeout(ctx, 5*time.Second)
		defer cancelTimeout()
		var wg wait.Group
		wg.Start(func() {
			_, err = multiStore.AwaitObject(ctxTimeout, configMapGvk, useNamespace, mapName)
		})
		time.Sleep(1 * time.Second)
		assert.True(t, multiStore.RemoveInformer(configMapGvk))
		wg.Wait()
		require.Equal(t, store.ErrInformerRemoved, err)
	})
	t.Run("missing informer", func(t *testing.T) {
		mapName := "i-do-not-exist-234234"
		ctxTimeout, cancelTimeout := context.WithTimeout(ctx, 5*time.Second)
		defer cancelTimeout()

		_, err := multiStore.AwaitObject(ctxTimeout, api_v1.SchemeGroupVersion.WithKind("Secret"), useNamespace, mapName)
		require.EqualError(t, err, "no informer for /v1, Kind=Secret is registered")
	})
}
