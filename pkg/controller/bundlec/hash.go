package bundlec

import (
	"hash"
	"io"
	"sort"

	"github.com/pkg/errors"
	core_v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
)

func (st *resourceSyncTask) hashSecretRef(name string, filter sets.String, optional *bool, h hash.Hash) error {
	secret, exists, err := st.derefObject(core_v1.SchemeGroupVersion.WithKind("Secret"), name)
	if err != nil {
		return err
	}
	if !exists {
		if optional == nil || !*optional {
			return errors.Errorf("Secret %q not found", name)
		}
		return nil
	}

	found := hashSecret(secret.(*core_v1.Secret), h, filter)
	if !found && (optional == nil || !*optional) {
		return errors.Errorf("not all keys %v found in Secret %q", filter, name)
	}

	return nil
}

func (st *resourceSyncTask) hashConfigMapRef(name string, filter sets.String, optional *bool, h hash.Hash) error {
	configmap, exists, err := st.derefObject(core_v1.SchemeGroupVersion.WithKind("ConfigMap"), name)
	if err != nil {
		return err
	}
	if !exists {
		if optional == nil || !*optional {
			return errors.Errorf("ConfigMap %q not found", name)
		}
		return nil
	}

	found := hashConfigMap(configmap.(*core_v1.ConfigMap), h, filter)
	if !found && (optional == nil || !*optional) {
		return errors.Errorf("not all keys %v found in ConfigMap %q", filter, name)
	}

	return nil
}

// hashConfigMap hashes the sorted values in the configmap in sorted order
// with a NUL as a separator character between and within pairs of key + value.
func hashConfigMap(configMap *core_v1.ConfigMap, h hash.Hash, filter sets.String) bool {
	keys := make([]string, 0, len(configMap.Data))
	search := sets.NewString(filter.UnsortedList()...)
	for k := range configMap.Data {
		if filter.Len() == 0 || filter.Has(k) {
			keys = append(keys, k)
		}
	}
	for k := range configMap.BinaryData {
		if filter.Len() == 0 || filter.Has(k) {
			keys = append(keys, k)
		}
	}
	search.Delete(keys...)
	if search.Len() != 0 {
		// not all the provided keys in filter were found
		return false
	}
	sort.Strings(keys)
	for _, k := range keys {
		io.WriteString(h, k) // nolint: gosec, errcheck
		h.Write([]byte{0})   // nolint: gosec, errcheck

		// The key is either in Data or BinaryData
		data, inData := configMap.Data[k]
		binaryData := configMap.BinaryData[k]
		if inData {
			io.WriteString(h, data) // nolint: gosec, errcheck
		} else {
			h.Write(binaryData) // nolint: gosec, errcheck
		}

		h.Write([]byte{0}) // nolint: gosec, errcheck
	}

	return true
}

// hashSecret hashes the sorted values in the secret in sorted order
// with a NUL as a separator character between and within pairs of key + value.
func hashSecret(secret *core_v1.Secret, h hash.Hash, filter sets.String) bool {
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
		return false
	}
	sort.Strings(keys)
	for _, k := range keys {
		io.WriteString(h, k)    // nolint: gosec, errcheck
		h.Write([]byte{0})      // nolint: gosec, errcheck
		h.Write(secret.Data[k]) // nolint: gosec, errcheck
		h.Write([]byte{0})      // nolint: gosec, errcheck
	}

	return true
}
