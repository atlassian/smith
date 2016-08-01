package main

func IsConflict(err error) bool {
	if status, ok := err.(*StatusError); ok {
		return status.status.Reason == StatusReasonConflict
	}
	return false
}
