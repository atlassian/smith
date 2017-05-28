// +build integration

package integration_tests

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/atlassian/smith/pkg/resources"
	"github.com/atlassian/smith/pkg/util"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/informers"
	api_v1 "k8s.io/client-go/pkg/api/v1"
)

func TestStore(t *testing.T) {
	_, clientset, _ := testSetup(t)

	store := resources.NewStore(resources.BundleScheme().DeepCopy)

	var wgStore sync.WaitGroup
	defer wgStore.Wait() // await store termination

	ctxStore, cancelStore := context.WithCancel(context.Background())
	defer cancelStore() // signal store to stop
	util.StartAsync(ctxStore, &wgStore, store.Run)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	configMapGvk := api_v1.SchemeGroupVersion.WithKind("ConfigMap")

	informerFactory := informers.NewSharedInformerFactory(clientset, 1*time.Minute)
	configMapInf := informerFactory.Core().V1().ConfigMaps().Informer()
	store.AddInformer(configMapGvk, configMapInf)
	informerFactory.Start(ctx.Done()) // Must be after store.AddInformer()

	t.Run("timeout", func(t *testing.T) {
		ctxTimeout, cancelTimeout1 := context.WithTimeout(ctx, 1*time.Second)
		defer cancelTimeout1()

		_, err := store.AwaitObject(ctxTimeout, configMapGvk, useNamespace, "i-do-not-exist-13123123123")
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
		if err != nil && !kerrors.IsNotFound(err) {
			require.NoError(t, err)
		}

		ctxTimeout, cancelTimeout2 := context.WithTimeout(ctx, 5*time.Second)
		defer cancelTimeout2()

		var obj runtime.Object
		var wg sync.WaitGroup
		util.StartAsync(ctxTimeout, &wg, func(ctx context.Context) {
			obj, err = store.AwaitObject(ctx, configMapGvk, useNamespace, mapName)
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
		var wg sync.WaitGroup
		util.StartAsync(ctxTimeout, &wg, func(ctx context.Context) {
			_, err = store.AwaitObject(ctx, configMapGvk, useNamespace, mapName)
		})
		time.Sleep(1 * time.Second)
		assert.True(t, store.RemoveInformer(configMapGvk))
		wg.Wait()
		require.Equal(t, resources.ErrInformerRemoved, err)
	})
	t.Run("missing informer", func(t *testing.T) {
		mapName := "i-do-not-exist-234234"
		ctxTimeout, cancelTimeout := context.WithTimeout(ctx, 5*time.Second)
		defer cancelTimeout()

		_, err := store.AwaitObject(ctxTimeout, api_v1.SchemeGroupVersion.WithKind("Secret"), useNamespace, mapName)
		require.EqualError(t, err, "no informer for /v1, Kind=Secret is registered")
	})
}
