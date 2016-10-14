package client

import (
	"strings"

	"github.com/atlassian/smith"
)

// IsAlreadyExists determines if the err is an error which indicates that a specified resource already exists.
func IsAlreadyExists(err error) bool {
	if status, ok := err.(*StatusError); ok {
		return status.status.Reason == smith.StatusReasonAlreadyExists
	}
	return false
}

// IsConflict determines if the err is an error which indicates the provided update conflicts.
func IsConflict(err error) bool {
	if status, ok := err.(*StatusError); ok {
		return status.status.Reason == smith.StatusReasonConflict
	}
	return false
}

// IsNotFound returns true if the specified error means that the resource was not found.
func IsNotFound(err error) bool {
	if status, ok := err.(*StatusError); ok {
		return status.status.Reason == smith.StatusReasonNotFound
	}
	return false
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
