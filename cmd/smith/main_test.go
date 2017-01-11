// +build integration

package main

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"testing"
	"time"

	"github.com/atlassian/smith"
	"github.com/atlassian/smith/pkg/resources"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/pkg/api/errors"
	"k8s.io/client-go/pkg/api/unversioned"
	apiv1 "k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/pkg/runtime"
	"k8s.io/client-go/pkg/watch"
)

func TestWorkflow(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)
	config := configFromEnv(t)

	templateClient, err := resources.GetTemplateTprClient(config)
	r.NoError(err)

	clients := dynamic.NewClientPool(config, nil, dynamic.LegacyAPIPathResolverFunc)

	templateName := "template1"
	templateNamespace := "default"

	var templateCreated bool
	tmpl := smith.Template{
		TypeMeta: unversioned.TypeMeta{
			Kind:       smith.TemplateResourceKind,
			APIVersion: smith.TemplateResourceGroupVersion,
		},
		Metadata: apiv1.ObjectMeta{
			Name: templateName,
		},
		Spec: smith.TemplateSpec{
			Resources: tplResources(r),
		},
	}
	err = templateClient.Delete().
		Namespace(templateNamespace).
		Resource(smith.TemplateResourcePath).
		Name(templateName).
		Do().
		Error()
	if err == nil {
		log.Print("Template deleted")
	} else if !errors.IsNotFound(err) {
		r.NoError(err)
	}
	defer func() {
		if !templateCreated {
			log.Printf("Deleting template %s", templateName)
			a.NoError(templateClient.Delete().
				Namespace(templateNamespace).
				Resource(smith.TemplateResourcePath).
				Name(templateName).
				Do().
				Error())
			for _, resource := range tmpl.Spec.Resources {
				log.Printf("Deleting resource %s", resource.Spec.GetName())
				gv, err := unversioned.ParseGroupVersion(resource.Spec.GetAPIVersion())
				if !a.NoError(err) {
					continue
				}
				client, err := clients.ClientForGroupVersionKind(gv.WithKind(resource.Spec.GetKind()))
				if !a.NoError(err) {
					continue
				}
				a.NoError(client.Resource(&unversioned.APIResource{
					Name:       resources.ResourceKindToPath(resource.Spec.GetKind()),
					Namespaced: true,
					Kind:       resource.Spec.GetKind(),
				}, templateNamespace).Delete(resource.Spec.GetName(), &apiv1.DeleteOptions{}))
			}
		}
	}()

	var wg sync.WaitGroup
	defer wg.Wait()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	wg.Add(1)
	go func() {
		defer wg.Done()

		if err := runWithConfig(ctx, config); err != nil && err != context.Canceled && err != context.DeadlineExceeded {
			t.Error(err)
		}
	}()

	time.Sleep(1 * time.Second) // Wait until the app starts and creates the Template TPR

	log.Print("Creating a new template")
	r.NoError(templateClient.Post().
		Namespace(templateNamespace).
		Resource(smith.TemplateResourcePath).
		Body(&tmpl).
		Do().
		Error())

	templateCreated = true

	for _, resource := range tmpl.Spec.Resources {
		func() {
			c, err := clients.ClientForGroupVersionKind(resource.Spec.GroupVersionKind())
			r.NoError(err)
			w, err := c.Resource(&unversioned.APIResource{
				Name:       resources.ResourceKindToPath(resource.Spec.GetKind()),
				Namespaced: true,
				Kind:       resource.Spec.GetKind(),
			}, templateNamespace).Watch(&apiv1.ListOptions{})
			r.NoError(err)
			defer w.Stop()
			ctxTimeout, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			for {
				select {
				case <-ctxTimeout.Done():
					t.Fatalf("Timeout waiting for events for resource %s", resource.Name)
				case ev := <-w.ResultChan():
					log.Printf("event %#v", ev)
					if ev.Type != watch.Added || ev.Object.GetObjectKind().GroupVersionKind() != resource.Spec.GetObjectKind().GroupVersionKind() {
						continue
					}
					obj, ok := ev.Object.(*runtime.Unstructured)
					r.True(ok)
					if obj.GetName() == resource.Spec.GetName() {
						log.Printf("received event for resource %q of kind %q", resource.Spec.GetName(), resource.Spec.GetKind())
						return
					}
				}
			}
		}()
	}
	time.Sleep(500 * time.Millisecond) // Wait a bit to let the server update the status
	var tmplRes smith.Template
	r.NoError(templateClient.Get().
		Namespace(templateNamespace).
		Resource(smith.TemplateResourcePath).
		Name(templateName).
		Do().
		Into(&tmplRes))
	log.Printf("tpl = %#v", &tmplRes)
	r.Equal(smith.READY, tmplRes.Status.State)
}

func tplResources(r *require.Assertions) []smith.Resource {
	c := apiv1.ConfigMap{
		TypeMeta: unversioned.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: "v1",
		},
		ObjectMeta: apiv1.ObjectMeta{
			Name: "config1",
		},
		Data: map[string]string{
			"a": "b",
		},
	}
	data, err := json.Marshal(&c)
	r.NoError(err)

	r1 := runtime.Unstructured{}
	r.NoError(r1.UnmarshalJSON(data))
	return []smith.Resource{
		{
			Name: "resource1",
			Spec: r1,
		},
	}
}
