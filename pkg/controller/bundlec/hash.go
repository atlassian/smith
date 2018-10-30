package bundlec

import (
	"hash"
	"sort"

	"github.com/pkg/errors"
	core_v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
)

func (st *resourceSyncTask) hashSecretRef(name, namespace string, filter sets.String, optional *bool, hash hash.Hash) error {
	secret, exists, err := st.derefObject(core_v1.SchemeGroupVersion.WithKind("Secret"), name, namespace)
	if err != nil {
		return err
	}
	if !exists {
		if optional == nil || !*optional {
			return errors.Errorf("secret %q not found", name)
		}
		return nil
	}

	found, err := hashSecret(secret.(*core_v1.Secret), hash, filter)
	if err != nil {
		return err
	}
	if !found && (optional == nil || !*optional) {
		return errors.Errorf("not all keys %v found in secret %q", filter, name)
	}

	return nil
}

func (st *resourceSyncTask) hashConfigMapRef(name, namespace string, filter sets.String, optional *bool, hash hash.Hash) error {
	configmap, exists, err := st.derefObject(core_v1.SchemeGroupVersion.WithKind("ConfigMap"), name, namespace)
	if err != nil {
		return err
	}
	if !exists {
		if optional == nil || !*optional {
			return errors.Errorf("configMap %q not found", name)
		}
		return nil
	}

	found, err := hashConfigMap(configmap.(*core_v1.ConfigMap), hash, filter)
	if err != nil {
		return err
	}
	if !found && (optional == nil || !*optional) {
		return errors.Errorf("not all keys %v found in configmap %q", filter, name)
	}

	return nil
}

// hashConfigMap hashes the sorted values in the configmap in sorted order
// with a NUL as a separator character between and within pairs of key + value.
func hashConfigMap(configMap *core_v1.ConfigMap, h hash.Hash, filter sets.String) (bool, error) {
	keys := make([]string, 0, len(configMap.Data))
	search := sets.NewString(filter.UnsortedList()...)
	for k := range configMap.Data {
		if filter.Len() == 0 || filter.Has(k) {
			keys = append(keys, k)
		}
	}
	search.Delete(keys...)
	if search.Len() != 0 {
		// not all the provided keys in filter were found
		return false, nil
	}
	sort.Strings(keys)
	for _, k := range keys {
		val := []byte(k)
		val = append(val, 0)
		val = append(val, []byte(configMap.Data[k])...)
		val = append(val, 0)
		_, err := h.Write(val)
		if err != nil {
			return false, errors.WithStack(err)
		}
	}

	return true, nil
}

// hashSecret hashes the sorted values in the secret in sorted order
// with a NUL as a separator character between and within pairs of key + value.
func hashSecret(secret *core_v1.Secret, h hash.Hash, filter sets.String) (bool, error) {
	keys := make([]string, 0, len(secret.Data))
	search := sets.NewString(filter.UnsortedList()...)
	for k := range secret.Data {
		if filter.Len() == 0 || filter.Has(k) {
			keys = append(keys, k)
		}
	}
	search.Delete(keys...)
	if search.Len() != 0 {
		// not all the provided keys in filter were found
		return false, nil
	}
	sort.Strings(keys)
	for _, k := range keys {
		val := []byte(k)
		val = append(val, 0)
		val = append(val, secret.Data[k]...)
		val = append(val, 0)
		_, err := h.Write(val)
		if err != nil {
			return false, errors.WithStack(err)
		}
	}

	return true, nil
}
