// +build integration

package main

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/atlassian/smith"
	"github.com/atlassian/smith/examples/tprattribute"
	"github.com/atlassian/smith/pkg/resources"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestTprAttribute(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)
	config := configFromEnv(t)

	tmplClient, _, err := resources.GetTemplateTprClient(config)
	r.NoError(err)

	sClient, _, err := tprattribute.GetSleeperTprClient(config)
	r.NoError(err)

	templateName := "template-attribute"
	templateNamespace := "default"

	var templateCreated bool
	sleeper, sleeperU := tmplAttrResources(r)
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
		r.NoError(err)
	}
	defer func() {
		if templateCreated {
			t.Logf("Deleting template %s", templateName)
			a.NoError(tmplClient.Delete().
				Namespace(templateNamespace).
				Resource(smith.TemplateResourcePath).
				Name(templateName).
				Do().
				Error())
			t.Logf("Deleting resource %s", sleeper.Metadata.Name)
			a.NoError(sClient.Delete().
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

		if err := runWithConfig(ctx, config); err != context.Canceled && err != context.DeadlineExceeded {
			a.NoError(err)
		}
	}()
	go func() {
		defer wg.Done()

		app := tprattribute.App{
			RestConfig: config,
		}
		if err := app.Run(ctx); err != context.Canceled && err != context.DeadlineExceeded {
			a.NoError(err)
		}
	}()

	time.Sleep(5 * time.Second) // Wait until apps start and creates the Template TPR and Sleeper TPR

	t.Log("Creating a new template")
	r.NoError(tmplClient.Post().
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
		r.NoError(err)
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
				a.EqualValues(map[string]string{
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
	r.NoError(tmplClient.Get().
		Namespace(templateNamespace).
		Resource(smith.TemplateResourcePath).
		Name(templateName).
		Do().
		Into(&tmplRes))
	r.Equal(smith.READY, tmplRes.Status.State)
}

func tmplAttrResources(r *require.Assertions) (*tprattribute.Sleeper, *unstructured.Unstructured) {
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
	r.NoError(err)

	u := &unstructured.Unstructured{}
	r.NoError(u.UnmarshalJSON(data))
	return c, u
}
