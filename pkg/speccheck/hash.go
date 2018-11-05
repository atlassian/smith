package speccheck

import (
	"hash"
	"io"
	"sort"

	"github.com/pkg/errors"
	core_v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
)

func HashSecretRef(store Store, namespace, name string, filter sets.String, optional *bool, h hash.Hash) error {
	secret, exists, err := store.Get(core_v1.SchemeGroupVersion.WithKind("Secret"), namespace, name)
	if err != nil {
		return errors.Wrapf(err, "failure retrieving Secret %q", name)
	}
	if !exists {
		if optional == nil || !*optional {
			return errors.Errorf("Secret %q not found", name)
		}
		return nil
	}

	found := HashSecret(secret.(*core_v1.Secret), h, filter)
	if !found && (optional == nil || !*optional) {
		return errors.Errorf("not all keys %v found in Secret %q", filter, name)
	}

	return nil
}

func HashConfigMapRef(store Store, namespace, name string, filter sets.String, optional *bool, h hash.Hash) error {
	configmap, exists, err := store.Get(core_v1.SchemeGroupVersion.WithKind("ConfigMap"), namespace, name)
	if err != nil {
		return errors.Wrapf(err, "failure retrieving ConfigMap %q", name)
	}
	if !exists {
		if optional == nil || !*optional {
			return errors.Errorf("ConfigMap %q not found", name)
		}
		return nil
	}

	found := HashConfigMap(configmap.(*core_v1.ConfigMap), h, filter)
	if !found && (optional == nil || !*optional) {
		return errors.Errorf("not all keys %v found in ConfigMap %q", filter, name)
	}

	return nil
}

// HashConfigMap hashes the sorted values in the ConfigMap in sorted order
// with a NUL as a separator character between and within pairs of key + value.
func HashConfigMap(configMap *core_v1.ConfigMap, h hash.Hash, filter sets.String) bool {
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

// HashSecret hashes the sorted values in the secret in sorted order
// with a NUL as a separator character between and within pairs of key + value.
func HashSecret(secret *core_v1.Secret, h hash.Hash, filter sets.String) bool {
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
