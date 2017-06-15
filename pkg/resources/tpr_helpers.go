package resources

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"reflect"
	"time"
	"unicode"

	"github.com/atlassian/smith"
	"github.com/atlassian/smith/pkg/util"

	api_errors "k8s.io/apimachinery/pkg/api/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	ext_v1b1 "k8s.io/client-go/pkg/apis/extensions/v1beta1"
)

func BundleTpr() *ext_v1b1.ThirdPartyResource {
	return &ext_v1b1.ThirdPartyResource{
		ObjectMeta: meta_v1.ObjectMeta{
			Name: smith.BundleResourceName,
		},
		Description: "Smith resource manager",
		Versions: []ext_v1b1.APIVersion{
			{Name: smith.BundleResourceVersion},
		},
	}
}

func GroupKindToTprName(gk schema.GroupKind) string {
	isFirst := true
	var buf bytes.Buffer
	for _, char := range gk.Kind {
		if unicode.IsUpper(char) {
			if isFirst {
				isFirst = false
			} else {
				buf.WriteByte('-')
			}
			buf.WriteRune(unicode.ToLower(char))
		} else {
			buf.WriteRune(char)
		}
	}
	buf.WriteByte('.')
	buf.WriteString(gk.Group)
	return buf.String()
}

func EnsureTprExists(ctx context.Context, clientset kubernetes.Interface, store smith.ByNameStore, tpr *ext_v1b1.ThirdPartyResource) error {
	tprGVK := ext_v1b1.SchemeGroupVersion.WithKind("ThirdPartyResource")
	for {
		obj, exists, err := store.Get(tprGVK, meta_v1.NamespaceNone, tpr.Name)
		if err != nil {
			return err
		}
		if exists {
			o := obj.(*ext_v1b1.ThirdPartyResource)
			// Ignoring labels and annotations for now
			if o.Description != tpr.Description ||
				!reflect.DeepEqual(o.Versions, tpr.Versions) ||
				!reflect.DeepEqual(o.Annotations, tpr.Annotations) ||
				!reflect.DeepEqual(o.Labels, tpr.Labels) {
				log.Printf("Updating ThirdPartyResource %s", tpr.Name)
				o.Description = tpr.Description
				o.Versions = tpr.Versions
				o.Annotations = tpr.Annotations
				o.Labels = tpr.Labels
				_, err = clientset.ExtensionsV1beta1().ThirdPartyResources().Update(o) // This is a CAS
				if err != nil {
					if !api_errors.IsConflict(err) {
						return fmt.Errorf("failed to update ThirdPartyResource %s: %v", tpr.Name, err)
					}
					log.Printf("Conflict updating ThirdPartyResource %s", tpr.Name)
					// wait for store to pick up the object and re-iterate
					if err := util.Sleep(ctx, 1*time.Second); err != nil {
						return err
					}
					continue
				}
			}
		} else {
			log.Printf("Creating ThirdPartyResource %s", tpr.Name)
			_, err := clientset.ExtensionsV1beta1().ThirdPartyResources().Create(tpr)
			if err != nil {
				if !api_errors.IsAlreadyExists(err) {
					return fmt.Errorf("failed to create %s ThirdPartyResource: %v", tpr.Name, err)
				}
				log.Printf("ThirdPartyResource %s was created concurrently", tpr.Name)
				// wait for store to pick up the object and re-iterate
				if err := util.Sleep(ctx, 1*time.Second); err != nil {
					return err
				}
				continue
			}
			log.Printf("ThirdPartyResource %s created", tpr.Name)
			// TODO It takes a while for k8s to add a new rest endpoint. Polling?
			if err := util.Sleep(ctx, 15*time.Second); err != nil {
				return err
			}
		}
		break
	}
	return nil
}
