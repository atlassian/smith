package bundlec

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/atlassian/smith"
	"github.com/atlassian/smith/pkg/util"
	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
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
//
// The annotation is only ever added to the spec object not the actual object. The modified spec is treated as
// if the user manually set the annotation.
//
// If the a user actually did set the annotation manually there are two cases.
// 1. if there are no parametersFrom referenced secrets we leave the spec untouched
// 2. if there are referenced secrets we proceed to check and generate a new value for the annotation
//
// This can be disabled by adding the annotation with the value 'disabled'
func (st *resourceSyncTask) processServiceInstance(spec *unstructured.Unstructured, actual runtime.Object, namespace string) (specRet *unstructured.Unstructured, e error) {
	instanceSpec := &sc_v1b1.ServiceInstance{}
	var previousEncodedChecksum string
	var updateCount int64

	if err := util.ConvertType(st.scheme, spec, instanceSpec); err != nil {
		return nil, errors.Wrap(err, "failure to parse ServiceInstance")
	}

	if len(instanceSpec.Spec.ParametersFrom) == 0 {
		return spec, nil
	}

	if v, ok := instanceSpec.ObjectMeta.Annotations[secretParametersChecksumAnnotation]; ok && v == disabled {
		return spec, nil
	}

	if actual != nil {
		actualInstance, ok := actual.(*sc_v1b1.ServiceInstance)
		if !ok {
			return nil, errors.New("failure retrieving ServiceInstance spec")
		}
		previousEncodedChecksum = actualInstance.ObjectMeta.Annotations[secretParametersChecksumAnnotation]
		updateCount = actualInstance.Spec.UpdateRequests
	}

	checkSum, err := st.calculateNewServiceInstanceCheckSum(instanceSpec, namespace)
	if err != nil {
		return nil, errors.Wrap(err, "failed to generate new checksum")
	}

	if actual != nil && checkSum != previousEncodedChecksum {
		instanceSpec.Spec.UpdateRequests = updateCount + 1
	}

	setInstanceAnnotation(instanceSpec, checkSum)

	unstructuredSpec, err := util.RuntimeToUnstructured(instanceSpec)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal back into unstructured")
	}

	return unstructuredSpec, nil
}

func (st *resourceSyncTask) calculateNewServiceInstanceCheckSum(instanceSpec *sc_v1b1.ServiceInstance, namespace string) (string, error) {
	checksumPayload, err := st.generateSecretParametersChecksumPayload(&instanceSpec.Spec, namespace)
	if err != nil {
		return "", errors.Wrap(err, "failed to generate checksum")
	}

	return hex.EncodeToString(checksumPayload), nil
}

func (st *resourceSyncTask) generateSecretParametersChecksumPayload(spec *sc_v1b1.ServiceInstanceSpec, namespace string) ([]byte, error) {
	hash := sha256.New()

	for _, parametersFrom := range spec.ParametersFrom {
		secretKeyRef := parametersFrom.SecretKeyRef
		err := st.hashSecretRef(secretKeyRef.Name, namespace, sets.NewString(secretKeyRef.Key), nil, hash)
		if err != nil {
			return nil, err
		}
	}

	return hash.Sum(nil), nil
}

func setInstanceAnnotation(instanceSpec *sc_v1b1.ServiceInstance, checksum string) {
	if instanceSpec.ObjectMeta.Annotations == nil {
		instanceSpec.ObjectMeta.Annotations = make(map[string]string, 1)
	}

	instanceSpec.ObjectMeta.Annotations[secretParametersChecksumAnnotation] = checksum
}
