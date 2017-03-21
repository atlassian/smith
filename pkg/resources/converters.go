package resources

import (
	"encoding/json"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// splitTprName splits TPR's name into resource name, group name and kind.
// e.g. "postgresql-resource.smith-sql.atlassian.com" is split into "postgresqlresources" and "smith-sql.atlassian.com".
// See https://github.com/kubernetes/kubernetes/blob/master/docs/design/extending-api.md
// See k8s.io/pkg/api/meta/restmapper.go:147 KindToResource()
func SplitTprName(name string) (string, *schema.GroupKind) {
	pos := strings.IndexByte(name, '.')
	if pos <= 0 {
		panic(fmt.Errorf("invalid resource name: %q", name))
	}
	resourcePath := strings.Replace(name[:pos], "-", "", -1)
	return ResourceKindToPath(resourcePath), &schema.GroupKind{
		Group: name[pos+1:],
		Kind:  strings.ToUpper(resourcePath[:1]) + resourcePath[1:],
	}
}

func ResourceKindToPath(kind string) string {
	kind = strings.ToLower(kind)
	switch kind[len(kind)-1] {
	case 's':
		kind += "es"
	case 'y':
		kind = kind[:len(kind)-1] + "ies"
	default:
		kind += "s"
	}
	return kind
}

func UnstructuredToType(obj *unstructured.Unstructured, t interface{}) error {
	data, err := obj.MarshalJSON()
	if err != nil {
		return err
	}
	return json.Unmarshal(data, t)
}
