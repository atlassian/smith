package app

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"sync"

	"github.com/ash2k/smith"
	"github.com/ash2k/smith/pkg/client"
	"github.com/ash2k/smith/pkg/processor"
)

type App struct {
	Version   string
	GitCommit string
}

func (a *App) Run(ctx context.Context) error {
	c, err := client.NewInCluster()
	if err != nil {
		return err
	}
	c.Agent = "smith/" + a.Version + "/" + a.GitCommit

	// 1. Ensure ThirdPartyResource TEMPLATE exists
	err = retryUntilSuccessOrDone(ctx, func() error {
		return ensureResourceExists(ctx, c)
	}, func(e error) bool {
		// TODO be smarter about what is retried
		log.Printf("Failed to create resource %s: %v", smith.TemplateResourceName, e)
		return false
	})
	if err != nil {
		return err
	}

	// 2. List existing templates
	var templateResources *smith.TemplateList
	err = retryUntilSuccessOrDone(ctx, func() error {
		templateResources = &smith.TemplateList{}
		return c.List(ctx, smith.TemplateResourceGroupVersion, smith.AllNamespaces, smith.AllResources, nil, nil, templateResources)
	}, func(e error) bool {
		// TODO be smarter about what is retried
		log.Printf("Failed to list resources %s: %v", smith.TemplateResourceName, e)
		return false
	})
	if err != nil {
		return err
	}

	// 3. Start rebuilds for existing templates to re-assert their state
	tp := processor.New(c)
	for _, template := range templateResources.Items {
		tCopy := template
		tp.Rebuild(&tCopy)
	}

	// 4. Watch for template-related events and trigger rebuilds as necessary
	// TODO also watch for events to supported resource types.
	// List TPRs to find out which ones to watch (inspect annotations?), have a hardcoded list of supported built-in resources
	var listMutex sync.Mutex // protects the listResVersion var
	listResVersion := templateResources.ResourceVersion
	events := c.Watch(ctx, smith.TemplateResourceGroupVersion, smith.AllNamespaces, smith.AllResources, newTemplateEvent, func() (interface{}, url.Values) {
		listMutex.Lock()
		lrv := listResVersion
		listMutex.Unlock()
		args := url.Values{}
		args.Set("resourceVersion", lrv)
		return nil, args
	})

	for event := range events {
		if e, ok := event.(error); ok {
			log.Printf("Something went wrong with watch: %v", e)
			continue
		}
		twe := event.(*smith.TemplateWatchEvent)
		switch twe.Type {
		case smith.Added, smith.Modified:
			tp.Rebuild(twe.Object.(*smith.Template))
		case smith.Deleted:
			// TODO Somehow use finalizers to prevent direct deletion?
			// "No direct deletion" convention? Use ObjectMeta.DeletionTimestamp like Namespace does?
			// Somehow implement GC to do cleanup after template is deleted?
			// Maybe store template in annotation on each resource to help reconstruct the dependency graph for GC?
		case smith.Error:
			// TODO what to do with it?
			log.Printf("Watch returned an Error event: %#v", twe)
		}
	}
	return ctx.Err()
}

func newTemplateEvent() interface{} {
	return &smith.TemplateWatchEvent{}
}

func ensureResourceExists(ctx context.Context, c *client.ResourceClient) error {
	log.Printf("Creating ThirdPartyResource %s", smith.TemplateResourceName)
	res := &smith.ThirdPartyResource{}
	err := c.Create(ctx, smith.ThirdPartyResourceAPIVersion, "", "thirdpartyresources", &smith.ThirdPartyResource{
		TypeMeta: smith.TypeMeta{
			Kind:       "ThirdPartyResource",
			APIVersion: smith.ThirdPartyResourceAPIVersion,
		},
		ObjectMeta: smith.ObjectMeta{
			Name: smith.TemplateResourceName,
		},
		Description: "Smith resource manager",
		Versions: []smith.APIVersion{
			{Name: smith.TemplateResourceVersion},
		},
	}, res)
	if err != nil {
		log.Printf("%#v", err)
		if !client.IsConflict(err) {
			return fmt.Errorf("failed to create ThirdPartyResource: %v", err)
		}
		log.Printf("ThirdPartyResource %s already exists", smith.TemplateResourceName)
	} else {
		log.Printf("ThirdPartyResource %s created: %+v", smith.TemplateResourceName, res)
	}
	return nil
}
