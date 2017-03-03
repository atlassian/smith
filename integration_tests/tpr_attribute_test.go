// +build integration

package integration_tests

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/atlassian/smith"
	"github.com/atlassian/smith/examples/tprattribute"
	"github.com/atlassian/smith/pkg/app"
	"github.com/atlassian/smith/pkg/resources"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestTprAttribute(t *testing.T) {
	config := configFromEnv(t)

	tmplClient, _, err := resources.GetTemplateTprClient(config)
	require.NoError(t, err)

	sClient, _, err := tprattribute.GetSleeperTprClient(config)
	require.NoError(t, err)

	templateName := "template-attribute"
	templateNamespace := "default"

	var templateCreated bool
	sleeper, sleeperU := tmplAttrResources(t)
	tmpl := smith.Template{
		TypeMeta: metav1.TypeMeta{
			Kind:       smith.TemplateResourceKind,
			APIVersion: smith.TemplateResourceGroupVersion,
		},
		Metadata: metav1.ObjectMeta{
			Name: templateName,
		},
		Spec: smith.TemplateSpec{
			Resources: []smith.Resource{
				{
					Name: sleeper.Metadata.Name,
					Spec: *sleeperU,
				},
			},
		},
	}
	err = tmplClient.Delete().
		Namespace(templateNamespace).
		Resource(smith.TemplateResourcePath).
		Name(templateName).
		Do().
		Error()
	if err == nil {
		t.Log("Template deleted")
	} else if !errors.IsNotFound(err) {
		require.NoError(t, err)
	}
	defer func() {
		if templateCreated {
			t.Logf("Deleting template %s", templateName)
			assert.NoError(t, tmplClient.Delete().
				Namespace(templateNamespace).
				Resource(smith.TemplateResourcePath).
				Name(templateName).
				Do().
				Error())
			t.Logf("Deleting resource %s", sleeper.Metadata.Name)
			assert.NoError(t, sClient.Delete().
				Namespace(templateNamespace).
				Resource(tprattribute.SleeperResourcePath).
				Name(sleeper.Metadata.Name).
				Do().
				Error())
		}
	}()

	var wg sync.WaitGroup
	defer wg.Wait()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	wg.Add(2)
	go func() {
		defer wg.Done()

		apl := app.App{
			RestConfig: config,
		}
		if err := apl.Run(ctx); err != context.Canceled && err != context.DeadlineExceeded {
			assert.NoError(t, err)
		}
	}()
	go func() {
		defer wg.Done()

		apl := tprattribute.App{
			RestConfig: config,
		}
		if err := apl.Run(ctx); err != context.Canceled && err != context.DeadlineExceeded {
			assert.NoError(t, err)
		}
	}()

	time.Sleep(5 * time.Second) // Wait until apps start and creates the Template TPR and Sleeper TPR

	t.Log("Creating a new template")
	require.NoError(t, tmplClient.Post().
		Namespace(templateNamespace).
		Resource(smith.TemplateResourcePath).
		Body(&tmpl).
		Do().
		Error())

	templateCreated = true

	func() {
		w, err := sClient.Get().
			Namespace(templateNamespace).
			Prefix("watch").
			Resource(tprattribute.SleeperResourcePath).
			Watch()
		require.NoError(t, err)
		defer w.Stop()
		ctxTimeout, cancel := context.WithTimeout(ctx, time.Duration(sleeper.Spec.SleepFor+1)*time.Second)
		defer cancel()
		for {
			select {
			case <-ctxTimeout.Done():
				t.Fatalf("Timeout waiting for events for resource %q", sleeper.Metadata.Name)
			case ev := <-w.ResultChan():
				t.Logf("event %#v", ev)
				obj := ev.Object.(*tprattribute.Sleeper)
				if obj.Metadata.Name != sleeper.Metadata.Name {
					continue
				}
				t.Logf("received event with status.state == %q for resource %q of kind %q", obj.Status.State, sleeper.Metadata.Name, sleeper.Kind)
				assert.EqualValues(t, map[string]string{
					smith.TemplateNameLabel: templateName,
				}, obj.Metadata.Labels)
				if obj.Status.State == tprattribute.AWAKE {
					return
				}
			}
		}
	}()
	time.Sleep(500 * time.Millisecond) // Wait a bit to let the server update the status
	var tmplRes smith.Template
	require.NoError(t, tmplClient.Get().
		Namespace(templateNamespace).
		Resource(smith.TemplateResourcePath).
		Name(templateName).
		Do().
		Into(&tmplRes))
	require.Equal(t, smith.READY, tmplRes.Status.State)
}

func tmplAttrResources(t *testing.T) (*tprattribute.Sleeper, *unstructured.Unstructured) {
	c := &tprattribute.Sleeper{
		TypeMeta: metav1.TypeMeta{
			Kind:       tprattribute.SleeperResourceKind,
			APIVersion: tprattribute.SleeperResourceGroupVersion,
		},
		Metadata: metav1.ObjectMeta{
			Name: "sleeper1",
		},
		Spec: tprattribute.SleeperSpec{
			SleepFor:      3, // seconds,
			WakeupMessage: "Hello, Infravators!",
		},
	}
	data, err := json.Marshal(c)
	require.NoError(t, err)

	u := &unstructured.Unstructured{}
	require.NoError(t, u.UnmarshalJSON(data))
	return c, u
}
