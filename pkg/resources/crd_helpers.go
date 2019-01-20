package resources

import (
	"context"
	"encoding/json"
	"io"
	"reflect"
	"time"

	ctrlLogz "github.com/atlassian/ctrl/logz"
	"github.com/atlassian/smith/pkg/util"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	apiext_v1b1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	apiExtClientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apiext_lst_v1b1 "k8s.io/apiextensions-apiserver/pkg/client/listers/apiextensions/v1beta1"
	api_errors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/yaml"
)

func PrintCleanedObject(sink io.Writer, format string, obj runtime.Object) error {
	u, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return errors.Wrap(err, "failed to convert objed to unstructured")
	}
	unstructured.RemoveNestedField(u, "metadata", "creationTimestamp")
	unstructured.RemoveNestedField(u, "status")
	switch format {
	case "json":
		enc := json.NewEncoder(sink)
		enc.SetIndent("", "  ")
		enc.SetEscapeHTML(false)
		err := enc.Encode(u)
		return errors.Wrap(err, "failed to marshal into JSON")
	case "yaml":
		data, err := yaml.Marshal(u)
		if err != nil {
			return errors.Wrap(err, "failed to marshal into YAML")
		}
		_, err = sink.Write(data)
		return errors.Wrap(err, "failed to write to sink")
	default:
		return errors.Errorf("unsupported output format %q", format)
	}
}

func EnsureCrdExistsAndIsEstablished(ctx context.Context, logger *zap.Logger, apiExtClient apiExtClientset.Interface, crdLister apiext_lst_v1b1.CustomResourceDefinitionLister, crd *apiext_v1b1.CustomResourceDefinition) error {
	err := EnsureCrdExists(ctx, logger, apiExtClient, crdLister, crd)
	if err != nil {
		return err
	}
	logger.Info("Waiting for CustomResourceDefinition to become established", ctrlLogz.Object(crd))
	return WaitForCrdToBecomeEstablished(ctx, crdLister, crd)
}

func EnsureCrdExists(ctx context.Context, logger *zap.Logger, apiExtClient apiExtClientset.Interface, crdLister apiext_lst_v1b1.CustomResourceDefinitionLister, crd *apiext_v1b1.CustomResourceDefinition) error {
	for {
		obj, err := crdLister.Get(crd.Name)
		notFound := api_errors.IsNotFound(err)
		if err != nil && !notFound {
			return err
		}
		if notFound {
			logger.Info("Creating CustomResourceDefinition", ctrlLogz.Object(crd))
			_, err = apiExtClient.ApiextensionsV1beta1().CustomResourceDefinitions().Create(crd)
			if err == nil {
				logger.Info("CustomResourceDefinition created", ctrlLogz.Object(crd))
				return nil
			}
			if !api_errors.IsAlreadyExists(err) {
				return errors.Wrapf(err, "failed to create %s CustomResourceDefinition", crd.Name)
			}
			logger.Info("CustomResourceDefinition was created concurrently", ctrlLogz.Object(crd))
		} else {
			if IsEqualCrd(crd, obj) {
				return nil
			}
			logger.Info("Updating CustomResourceDefinition", ctrlLogz.Object(crd))
			obj = obj.DeepCopy()
			obj.Spec = crd.Spec
			obj.Annotations = crd.Annotations
			obj.Labels = crd.Labels
			// TODO erasing the status is only necessary because there is no support for generation/observedGeneration at the moment
			obj.Status = apiext_v1b1.CustomResourceDefinitionStatus{}
			_, err = apiExtClient.ApiextensionsV1beta1().CustomResourceDefinitions().Update(obj) // This is a CAS
			if err == nil {
				logger.Info("CustomResourceDefinition updated", ctrlLogz.Object(crd))
				return nil
			}
			if !api_errors.IsConflict(err) {
				return errors.Wrapf(err, "failed to update CustomResourceDefinition %s", crd.Name)
			}
			logger.Info("Conflict updating CustomResourceDefinition, retrying", ctrlLogz.Object(crd))
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

func IsEqualCrd(a, b *apiext_v1b1.CustomResourceDefinition) bool {
	a = a.DeepCopy()
	b = b.DeepCopy()

	apiext_v1b1.SetDefaults_CustomResourceDefinitionSpec(&a.Spec)
	apiext_v1b1.SetDefaults_CustomResourceDefinitionSpec(&b.Spec)

	// Ignoring labels
	as := a.Spec
	bs := b.Spec
	return as.Group == bs.Group &&
		isEqualCrdNames(as.Names, bs.Names) &&
		as.Scope == bs.Scope &&
		isEqualValidation(as.Validation, bs.Validation) &&
		isEqualSubresources(as.Subresources, bs.Subresources) &&
		isEqualVersions(as.Versions, bs.Versions) &&
		isEqualAdditionalPrinterColumns(as.AdditionalPrinterColumns, bs.AdditionalPrinterColumns) &&
		isEqualAnnotations(a.Annotations, b.Annotations)
}

func isEqualCrdNames(a, b apiext_v1b1.CustomResourceDefinitionNames) bool {
	return reflect.DeepEqual(a, b)
}

func isEqualValidation(a, b *apiext_v1b1.CustomResourceValidation) bool {
	return reflect.DeepEqual(a, b)
}

func isEqualSubresources(a, b *apiext_v1b1.CustomResourceSubresources) bool {
	return reflect.DeepEqual(a, b)
}

func isEqualVersions(a, b []apiext_v1b1.CustomResourceDefinitionVersion) bool {
	return reflect.DeepEqual(a, b)
}

func isEqualAdditionalPrinterColumns(a, b []apiext_v1b1.CustomResourceColumnDefinition) bool {
	return reflect.DeepEqual(a, b)
}

func isEqualAnnotations(a, b map[string]string) bool {
	return reflect.DeepEqual(a, b)
}
