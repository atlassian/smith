// +build integration

package main

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/atlassian/smith"
	"github.com/atlassian/smith/pkg/resources"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	apiv1 "k8s.io/client-go/pkg/api/v1"
)

func TestWorkflow(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)
	config := configFromEnv(t)

	tmplClient, _, err := resources.GetTemplateTprClient(config)
	r.NoError(err)

	clients := dynamic.NewClientPool(config, nil, dynamic.LegacyAPIPathResolverFunc)

	templateName := "template1"
	templateNamespace := "default"

	var templateCreated bool
	tmpl := smith.Template{
		TypeMeta: metav1.TypeMeta{
			Kind:       smith.TemplateResourceKind,
			APIVersion: smith.TemplateResourceGroupVersion,
		},
		Metadata: metav1.ObjectMeta{
			Name: templateName,
			Labels: map[string]string{
				"templateLabel":         "templateValue",
				"overlappingLabel":      "overlappingTemplateValue",
				smith.TemplateNameLabel: "templateLabel123",
			},
		},
		Spec: smith.TemplateSpec{
			Resources: tmplResources(r),
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
			for _, resource := range tmpl.Spec.Resources {
				t.Logf("Deleting resource %s", resource.Spec.GetName())
				gv, err := schema.ParseGroupVersion(resource.Spec.GetAPIVersion())
				if !a.NoError(err) {
					continue
				}
				client, err := clients.ClientForGroupVersionKind(gv.WithKind(resource.Spec.GetKind()))
				if !a.NoError(err) {
					continue
				}
				a.NoError(client.Resource(&metav1.APIResource{
					Name:       resources.ResourceKindToPath(resource.Spec.GetKind()),
					Namespaced: true,
					Kind:       resource.Spec.GetKind(),
				}, templateNamespace).Delete(resource.Spec.GetName(), &metav1.DeleteOptions{}))
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

		if err := runWithConfig(ctx, config); err != context.Canceled && err != context.DeadlineExceeded {
			a.NoError(err)
		}
	}()

	time.Sleep(1 * time.Second) // Wait until the app starts and creates the Template TPR

	t.Log("Creating a new template")
	var tmplRes smith.Template
	r.NoError(tmplClient.Post().
		Namespace(templateNamespace).
		Resource(smith.TemplateResourcePath).
		Body(&tmpl).
		Do().
		Into(&tmplRes))

	templateCreated = true

	for _, resource := range tmpl.Spec.Resources {
		func() {
			c, err := clients.ClientForGroupVersionKind(resource.Spec.GroupVersionKind())
			r.NoError(err)
			w, err := c.Resource(&metav1.APIResource{
				Name:       resources.ResourceKindToPath(resource.Spec.GetKind()),
				Namespaced: true,
				Kind:       resource.Spec.GetKind(),
			}, templateNamespace).Watch(metav1.ListOptions{})
			r.NoError(err)
			defer w.Stop()
			ctxTimeout, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			for {
				select {
				case <-ctxTimeout.Done():
					t.Fatalf("Timeout waiting for events for resource %s", resource.Name)
				case ev := <-w.ResultChan():
					t.Logf("event %#v", ev)
					if ev.Type != watch.Added || ev.Object.GetObjectKind().GroupVersionKind() != resource.Spec.GroupVersionKind() {
						continue
					}
					obj := ev.Object.(*unstructured.Unstructured)
					if obj.GetName() != resource.Spec.GetName() {
						continue
					}
					t.Logf("received event for resource %q of kind %q", resource.Spec.GetName(), resource.Spec.GetKind())
					a.Equal(map[string]string{
						"configLabel":           "configValue",
						"templateLabel":         "templateValue",
						"overlappingLabel":      "overlappingConfigValue",
						smith.TemplateNameLabel: templateName,
					}, obj.GetLabels())
					a.Equal([]metav1.OwnerReference{
						{
							APIVersion: smith.TemplateResourceVersion,
							Kind:       smith.TemplateResourceKind,
							Name:       templateName,
							UID:        tmplRes.Metadata.UID,
						},
					}, obj.GetOwnerReferences())
					return
				}
			}
		}()
	}
	time.Sleep(500 * time.Millisecond) // Wait a bit to let the server update the status
	r.NoError(tmplClient.Get().
		Namespace(templateNamespace).
		Resource(smith.TemplateResourcePath).
		Name(templateName).
		Do().
		Into(&tmplRes))
	r.Equal(smith.READY, tmplRes.Status.State, "%#v", tmplRes)
}

func tmplResources(r *require.Assertions) []smith.Resource {
	c := apiv1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "config1",
			Labels: map[string]string{
				"configLabel":           "configValue",
				"overlappingLabel":      "overlappingConfigValue",
				smith.TemplateNameLabel: "configLabel123",
			},
		},
		Data: map[string]string{
			"a": "b",
		},
	}
	data, err := json.Marshal(&c)
	r.NoError(err)

	r1 := unstructured.Unstructured{}
	r.NoError(r1.UnmarshalJSON(data))
	return []smith.Resource{
		{
			Name: "resource1",
			Spec: r1,
		},
	}
}
