package client

import (
	"strings"
)

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
