// +build integration

package integration_tests

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/atlassian/smith/pkg/resources"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	apiv1 "k8s.io/client-go/pkg/api/v1"
)

func TestStore(t *testing.T) {
	config, err := resources.ConfigFromEnv()
	require.NoError(t, err)

	clientset, err := kubernetes.NewForConfig(config)
	require.NoError(t, err)

	store := resources.NewStore(smithScheme().DeepCopy)

	var wgStore sync.WaitGroup
	defer wgStore.Wait() // await store termination

	ctxStore, cancelStore := context.WithCancel(context.Background())
	defer cancelStore() // signal store to stop
	wgStore.Add(1)
	go store.Run(ctxStore, &wgStore)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	configMapGvk := apiv1.SchemeGroupVersion.WithKind("ConfigMap")

	informerFactory := informers.NewSharedInformerFactory(clientset, 1*time.Minute)
	configMapInf := informerFactory.Core().V1().ConfigMaps().Informer()
	store.AddInformer(configMapGvk, configMapInf)
	informerFactory.Start(ctx.Done()) // Must be after store.AddInformer()

	t.Run("timeout", func(t *testing.T) {
		ctxTimeout, cancelTimeout1 := context.WithTimeout(ctx, 5*time.Second)
		defer cancelTimeout1()

		_, err = store.AwaitObject(ctxTimeout, configMapGvk, useNamespace, "i-do-not-exist-13123123123")
		assert.EqualError(t, err, context.DeadlineExceeded.Error())
	})
	t.Run("create", func(t *testing.T) {
		mapName := "i-do-not-exist-yet"
		cm := &apiv1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name: mapName,
			},
			Data: map[string]string{
				"a": "b",
			},
		}

		err = clientset.CoreV1().ConfigMaps(useNamespace).Delete(mapName, &metav1.DeleteOptions{})
		if err != nil && !kerrors.IsNotFound(err) {
			require.NoError(t, err)
		}

		ctxTimeout, cancelTimeout2 := context.WithTimeout(ctx, 5*time.Second)
		defer cancelTimeout2()

		var obj runtime.Object
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			obj, err = store.AwaitObject(ctxTimeout, configMapGvk, useNamespace, mapName)
		}()
		time.Sleep(1 * time.Second)
		cm, errCreate := clientset.CoreV1().ConfigMaps(useNamespace).Create(cm)
		require.NoError(t, errCreate)
		defer func() {
			// Cleanup after successful create
			err = clientset.CoreV1().ConfigMaps(useNamespace).Delete(mapName, &metav1.DeleteOptions{})
			assert.NoError(t, err)
		}()
		wg.Wait()
		require.NoError(t, err)
		assert.Equal(t, cm, obj)
	})
	t.Run("remove informer", func(t *testing.T) {
		mapName := "i-do-not-exist-234234"
		ctxTimeout, cancelTimeout := context.WithTimeout(ctx, 5*time.Second)
		defer cancelTimeout()
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err = store.AwaitObject(ctxTimeout, configMapGvk, useNamespace, mapName)
		}()
		time.Sleep(1 * time.Second)
		assert.True(t, store.RemoveInformer(configMapGvk))
		wg.Wait()
		require.Equal(t, resources.ErrInformerRemoved, err)
	})
	t.Run("missing informer", func(t *testing.T) {
		mapName := "i-do-not-exist-234234"
		ctxTimeout, cancelTimeout := context.WithTimeout(ctx, 5*time.Second)
		defer cancelTimeout()

		_, err = store.AwaitObject(ctxTimeout, apiv1.SchemeGroupVersion.WithKind("Secret"), useNamespace, mapName)
		require.EqualError(t, err, "no informer for /v1, Kind=Secret is registered")
	})
}
