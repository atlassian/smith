package client

import (
	"github.com/ash2k/smith"
)

func IsConflict(err error) bool {
	if status, ok := err.(*StatusError); ok {
		return status.status.Reason == smith.StatusReasonConflict
	}
	return false
}
