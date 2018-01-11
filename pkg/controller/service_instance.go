package controller

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"

	"github.com/atlassian/smith"
	"github.com/atlassian/smith/pkg/util"

	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	"github.com/pkg/errors"
	"golang.org/x/crypto/bcrypt"
	core_v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	unstructured_conversion "k8s.io/apimachinery/pkg/conversion/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	secretParametersChecksumAnnotation = smith.Domain + "/secretParametersChecksum"
	disabled                           = "disabled"
)

// works around the fact that service catalog does not know when to send an update request if the
// parameters section would change as the result of changing a secret referenced in parametersFrom. To
// do this we record the contents of all referenced secrets in an annotation, compare that annotations value
// each time a service instance is processed, and force UpdateRequests to a higher value when those secrets
// change to trigger service catalog to send an update request.
func (st *resourceSyncTask) forceServiceInstanceUpdates(spec *unstructured.Unstructured, actual runtime.Object, namespace string) (specRet *unstructured.Unstructured, e error) {
	gvk := spec.GroupVersionKind()

	if gvk.Group == sc_v1b1.GroupName && gvk.Kind == "ServiceInstance" {
		return st.processServiceInstance(spec, actual, namespace)
	}

	return spec, nil
}

// works by adding using an annotation to remember the previous contents of all the secrets referenced in
// parametersFrom and forcing updateRequest to higher value when the referenced secrets change. The annotation
// stores the hex value of the bcryted hash of the sha256 hash of the contents of all secrets referenced in
// parametersFrom.
//
// The annotation is only ever added to the spec object not the actual object. The modified spec is treated as
// if the user manually set the annotation.  This means that if the spec is updated to remove all parametersFrom
// referenced secrets the annotation will be removed.
//
// If the a user actually did set the annotation manually there are two cases.
// 1. if there are no parametersFrom referenced secrets we leave the spec untouched
// 2. if there are referenced secrets we proceed to check and generate a new value for the annotation
//
// This can be disabled by adding the annotation with the value 'disabled'
func (st *resourceSyncTask) processServiceInstance(spec *unstructured.Unstructured, actual runtime.Object, namespace string) (specRet *unstructured.Unstructured, e error) {
	var instanceSpec = &sc_v1b1.ServiceInstance{}
	var previousEncodedChecksum string
	var updateCount int64

	if err := unstructured_conversion.DefaultConverter.FromUnstructured(spec.Object, instanceSpec); err != nil {
		return nil, errors.Wrap(err, "failure to parse ServiceInstance")
	}

	if len(instanceSpec.Spec.ParametersFrom) == 0 {
		return spec, nil
	}

	if v, ok := instanceSpec.ObjectMeta.Annotations[secretParametersChecksumAnnotation]; ok && v == disabled {
		return spec, nil
	}

	if actual != nil {
		spec, ok := actual.(*sc_v1b1.ServiceInstance)
		if !ok {
			return nil, errors.New("failure retrieving ServiceInstance spec")
		}
		previousEncodedChecksum = spec.ObjectMeta.Annotations[secretParametersChecksumAnnotation]
		updateCount = spec.Spec.UpdateRequests
	}

	checkSum, err := st.calculateNewCheckSum(instanceSpec, namespace, previousEncodedChecksum)
	if err != nil {
		return nil, errors.Wrap(err, "failed to generate new checksum")
	}

	if actual != nil && checkSum != previousEncodedChecksum {
		instanceSpec.Spec.UpdateRequests = updateCount + 1
	}

	setAnnotation(instanceSpec, checkSum)

	unstructuredSpec, err := util.RuntimeToUnstructured(instanceSpec)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal back into unstructured")
	}

	return unstructuredSpec, nil
}

func (st *resourceSyncTask) calculateNewCheckSum(instanceSpec *sc_v1b1.ServiceInstance, namespace string, previousEncodedChecksum string) (string, error) {
	checksumPayload, err := st.generateSecretParametersChecksumPayload(&instanceSpec.Spec, namespace)
	if err != nil {
		return "", errors.Wrap(err, "failed to generate checksum")
	}

	if previousEncodedChecksum != "" {
		previousCheckSum, err := decodeChecksum(previousEncodedChecksum)
		if err == nil && validateChecksum(previousCheckSum, checksumPayload) {
			return previousEncodedChecksum, nil
		}
	}
	checksum, err := generateChecksum(checksumPayload)
	if err != nil {
		return "", err
	}

	return encodeChecksum(checksum), nil
}

func (st *resourceSyncTask) generateSecretParametersChecksumPayload(spec *sc_v1b1.ServiceInstanceSpec, namespace string) ([]byte, error) {
	var buf bytes.Buffer

	for _, parametersFrom := range spec.ParametersFrom {
		secretKeyRef := parametersFrom.SecretKeyRef
		secretObj, exists, err := st.store.Get(core_v1.SchemeGroupVersion.WithKind("Secret"), namespace, secretKeyRef.Name)
		if err != nil {
			return nil, errors.Wrapf(err, "failure retrieving secret %q referenced from spec.ParametersFrom", secretKeyRef.Name)
		}
		if !exists {
			return nil, errors.Errorf("secret %q referenced from spec.ParametersFrom not found", secretKeyRef.Name)
		}
		secret, ok := secretObj.(*core_v1.Secret)
		if !ok {
			return nil, errors.Errorf("failure casting secret %q referenced from spec.ParametersFrom", secretKeyRef.Name)
		}

		secretData := secret.Data[secretKeyRef.Key]
		if secretData == nil {
			return nil, errors.Errorf("key %q not found in Secret %q", secretKeyRef.Key, secretKeyRef.Name)
		}
		// Using SHA256 here is fine since we will hash the final result with bcrypt anyway
		hash := sha256.Sum256(secretData)
		_, err = buf.Write(hash[:])
		if err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

func setAnnotation(instanceSpec *sc_v1b1.ServiceInstance, checksum string) {
	if instanceSpec.ObjectMeta.Annotations == nil {
		instanceSpec.ObjectMeta.Annotations = make(map[string]string, 1)
	}

	instanceSpec.ObjectMeta.Annotations[secretParametersChecksumAnnotation] = checksum
}

func generateChecksum(data []byte) ([]byte, error) {
	return bcrypt.GenerateFromPassword(data, bcrypt.MinCost)
}

// hashed password with its possible plaintext equivalent, return true if they match
func validateChecksum(checksum, data []byte) bool {
	err := bcrypt.CompareHashAndPassword(checksum, data)
	return err == nil
}

func encodeChecksum(data []byte) string {
	return hex.EncodeToString(data)
}

func decodeChecksum(s string) ([]byte, error) {
	return hex.DecodeString(s)
}
