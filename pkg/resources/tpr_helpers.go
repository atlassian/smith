package resources

import (
	"bytes"
	"unicode"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

func GroupKindToTprName(gk schema.GroupKind) string {
	isFirst := true
	var buf bytes.Buffer
	for _, char := range gk.Kind {
		if unicode.IsUpper(char) {
			if isFirst {
				isFirst = false
			} else {
				buf.WriteByte('-')
			}
			buf.WriteRune(unicode.ToLower(char))
		} else {
			buf.WriteRune(char)
		}
	}
	buf.WriteByte('.')
	buf.WriteString(gk.Group)
	return buf.String()
}
