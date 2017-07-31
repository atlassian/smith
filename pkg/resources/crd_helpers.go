package resources

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/atlassian/smith"
	"github.com/atlassian/smith/pkg/util"

	apiext_v1b1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	crdClientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	api_errors "k8s.io/apimachinery/pkg/api/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
)

var (
	crdGVK = apiext_v1b1.SchemeGroupVersion.WithKind("CustomResourceDefinition")
)

func BundleCrd() *apiext_v1b1.CustomResourceDefinition {
	return &apiext_v1b1.CustomResourceDefinition{
		ObjectMeta: meta_v1.ObjectMeta{
			Name: smith.BundleResourceName,
		},
		Spec: apiext_v1b1.CustomResourceDefinitionSpec{
			Group:   smith.SmithResourceGroup,
			Version: smith.BundleResourceVersion,
			Names: apiext_v1b1.CustomResourceDefinitionNames{
				Plural:   smith.BundleResourcePlural,
				Singular: smith.BundleResourceSingular,
				Kind:     smith.BundleResourceKind,
			},
		},
	}
}

func EnsureCrdExists(ctx context.Context, scheme *runtime.Scheme, clientset crdClientset.Interface, store smith.ByNameStore, crd *apiext_v1b1.CustomResourceDefinition) error {
	for {
		obj, exists, err := store.Get(crdGVK, meta_v1.NamespaceNone, crd.Name)
		if err != nil {
			return err
		}
		if exists {
			o := obj.(*apiext_v1b1.CustomResourceDefinition)
			if IsEqualCrd(crd, o, scheme) {
				return nil
			}
			log.Printf("Updating CustomResourceDefinition %s", crd.Name)
			o.Spec = crd.Spec
			o.Annotations = crd.Annotations
			o.Labels = crd.Labels
			_, err = clientset.ApiextensionsV1beta1().CustomResourceDefinitions().Update(o) // This is a CAS
			if err == nil {
				log.Printf("CustomResourceDefinition %s updated, waiting for it to become established", crd.Name)
				return WaitForCrdToBecomeEstablished(ctx, store, crd)
			}
			if !api_errors.IsConflict(err) {
				return fmt.Errorf("failed to update CustomResourceDefinition %s: %v", crd.Name, err)
			}
			log.Printf("Conflict updating CustomResourceDefinition %s, retrying", crd.Name)
		} else {
			log.Printf("Creating CustomResourceDefinition %s", crd.Name)
			_, err := clientset.ApiextensionsV1beta1().CustomResourceDefinitions().Create(crd)
			if err == nil {
				log.Printf("CustomResourceDefinition %s created, waiting for it to become established", crd.Name)
				return WaitForCrdToBecomeEstablished(ctx, store, crd)
			}
			if !api_errors.IsAlreadyExists(err) {
				return fmt.Errorf("failed to create %s CustomResourceDefinition: %v", crd.Name, err)
			}
			log.Printf("CustomResourceDefinition %s was created concurrently", crd.Name)
		}
		// wait for store to pick up the object and re-iterate
		if err = util.Sleep(ctx, time.Second); err != nil {
			return err
		}
	}
}

func WaitForCrdToBecomeEstablished(ctx context.Context, store smith.ByNameStore, crd *apiext_v1b1.CustomResourceDefinition) error {
	return wait.PollUntil(100*time.Millisecond, func() (done bool, err error) {
		obj, exists, err := store.Get(crdGVK, meta_v1.NamespaceNone, crd.Name)
		if err != nil || !exists {
			return false, err
		}
		established := false
		for _, cond := range obj.(*apiext_v1b1.CustomResourceDefinition).Status.Conditions {
			switch cond.Type {
			case apiext_v1b1.Established:
				if cond.Status == apiext_v1b1.ConditionTrue {
					established = true
				}
			case apiext_v1b1.NamesAccepted:
				if cond.Status == apiext_v1b1.ConditionFalse {
					return false, fmt.Errorf("failed to create CRD %s: name conflict: %s", crd.Name, cond.Reason)
				}
			}
		}
		return established, nil
	}, ctx.Done())
}

func IsEqualCrd(a, b *apiext_v1b1.CustomResourceDefinition, scheme *runtime.Scheme) bool {
	aCopy := *a
	bCopy := *b
	a = &aCopy
	b = &bCopy

	scheme.Default(a)
	scheme.Default(b)

	// Ignoring labels and annotations for now
	as := a.Spec
	bs := b.Spec
	return as.Group == bs.Group &&
		as.Version == bs.Version &&
		isEqualCrdNames(a, b) &&
		a.Spec.Scope == b.Spec.Scope
}

func isEqualCrdNames(a, b *apiext_v1b1.CustomResourceDefinition) bool {
	n1 := a.Spec.Names
	n2 := b.Spec.Names
	return n1.Plural == n2.Plural &&
		n1.Singular == n2.Singular &&
		isEqualShortNames(n1.ShortNames, n2.ShortNames) &&
		n1.Kind == n2.Kind &&
		n1.ListKind == n2.ListKind
}

func isEqualShortNames(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	for i, v := range a {
		if v != b[i] {
			return false
		}
	}

	return true
}
