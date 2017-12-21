package resources

import (
	"context"
	"log"
	"time"

	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/smith/pkg/util"
	"github.com/pkg/errors"

	apiext_v1b1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	crdClientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apiext_lst_v1b1 "k8s.io/apiextensions-apiserver/pkg/client/listers/apiextensions/v1beta1"
	api_errors "k8s.io/apimachinery/pkg/api/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
)

func BundleCrd() *apiext_v1b1.CustomResourceDefinition {
	return &apiext_v1b1.CustomResourceDefinition{
		ObjectMeta: meta_v1.ObjectMeta{
			Name: smith_v1.BundleResourceName,
		},
		Spec: apiext_v1b1.CustomResourceDefinitionSpec{
			Group:   smith_v1.GroupName,
			Version: smith_v1.BundleResourceVersion,
			Names: apiext_v1b1.CustomResourceDefinitionNames{
				Plural:   smith_v1.BundleResourcePlural,
				Singular: smith_v1.BundleResourceSingular,
				Kind:     smith_v1.BundleResourceKind,
			},
		},
	}
}

func EnsureCrdExistsAndIsEstablished(ctx context.Context, defaulter runtime.ObjectDefaulter, clientset crdClientset.Interface, crdLister apiext_lst_v1b1.CustomResourceDefinitionLister, crd *apiext_v1b1.CustomResourceDefinition) error {
	err := EnsureCrdExists(ctx, defaulter, clientset, crdLister, crd)
	if err != nil {
		return err
	}
	log.Printf("Waiting for CustomResourceDefinition %s to become established", crd.Name)
	return WaitForCrdToBecomeEstablished(ctx, crdLister, crd)
}

func EnsureCrdExists(ctx context.Context, defaulter runtime.ObjectDefaulter, clientset crdClientset.Interface, crdLister apiext_lst_v1b1.CustomResourceDefinitionLister, crd *apiext_v1b1.CustomResourceDefinition) error {
	for {
		obj, err := crdLister.Get(crd.Name)
		notFound := api_errors.IsNotFound(err)
		if err != nil && !notFound {
			return err
		}
		if notFound {
			log.Printf("Creating CustomResourceDefinition %s", crd.Name)
			_, err = clientset.ApiextensionsV1beta1().CustomResourceDefinitions().Create(crd)
			if err == nil {
				log.Printf("CustomResourceDefinition %s created", crd.Name)
				return nil
			}
			if !api_errors.IsAlreadyExists(err) {
				return errors.Wrapf(err, "failed to create %s CustomResourceDefinition", crd.Name)
			}
			log.Printf("CustomResourceDefinition %s was created concurrently", crd.Name)
		} else {
			if IsEqualCrd(crd, obj, defaulter) {
				return nil
			}
			log.Printf("Updating CustomResourceDefinition %s", crd.Name)
			obj.Spec = crd.Spec
			obj.Annotations = crd.Annotations
			obj.Labels = crd.Labels
			// TODO erasing the status is only necessary because there is no support for generation/observedGeneration at the moment
			obj.Status = apiext_v1b1.CustomResourceDefinitionStatus{}
			_, err = clientset.ApiextensionsV1beta1().CustomResourceDefinitions().Update(obj) // This is a CAS
			if err == nil {
				log.Printf("CustomResourceDefinition %s updated", crd.Name)
				return nil
			}
			if !api_errors.IsConflict(err) {
				return errors.Wrapf(err, "failed to update CustomResourceDefinition %s", crd.Name)
			}
			log.Printf("Conflict updating CustomResourceDefinition %s, retrying", crd.Name)
		}
		// wait for store to pick up the object and re-iterate
		if err = util.Sleep(ctx, time.Second); err != nil {
			return err
		}
	}
}

func WaitForCrdToBecomeEstablished(ctx context.Context, crdLister apiext_lst_v1b1.CustomResourceDefinitionLister, crd *apiext_v1b1.CustomResourceDefinition) error {
	return wait.PollUntil(100*time.Millisecond, func() (done bool, err error) {
		obj, err := crdLister.Get(crd.Name)
		if err != nil {
			if api_errors.IsNotFound(err) {
				return false, nil
			}
			return false, err
		}
		// TODO check generation/observedGeneration when supported
		established := false
		for _, cond := range obj.Status.Conditions {
			switch cond.Type {
			case apiext_v1b1.Established:
				if cond.Status == apiext_v1b1.ConditionTrue {
					established = true
				}
			case apiext_v1b1.NamesAccepted:
				if cond.Status == apiext_v1b1.ConditionFalse {
					return false, errors.Errorf("failed to create CRD %s: name conflict: %s", crd.Name, cond.Reason)
				}
			}
		}
		return established, nil
	}, ctx.Done())
}

// IsCrdConditionTrue indicates if the condition is present and strictly true
func IsCrdConditionTrue(crd *apiext_v1b1.CustomResourceDefinition, conditionType apiext_v1b1.CustomResourceDefinitionConditionType) bool {
	return IsCrdConditionPresentAndEqual(crd, conditionType, apiext_v1b1.ConditionTrue)
}

// IsCrdConditionPresentAndEqual indicates if the condition is present and equal to the arg
func IsCrdConditionPresentAndEqual(crd *apiext_v1b1.CustomResourceDefinition, conditionType apiext_v1b1.CustomResourceDefinitionConditionType, status apiext_v1b1.ConditionStatus) bool {
	for _, condition := range crd.Status.Conditions {
		if condition.Type == conditionType {
			return condition.Status == status
		}
	}
	return false
}

func IsEqualCrd(a, b *apiext_v1b1.CustomResourceDefinition, defaulter runtime.ObjectDefaulter) bool {
	aCopy := *a
	bCopy := *b
	a = &aCopy
	b = &bCopy

	defaulter.Default(a)
	defaulter.Default(b)

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
