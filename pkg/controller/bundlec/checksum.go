package bundlec

import (
	"encoding/hex"

	"golang.org/x/crypto/bcrypt"
)

func generateChecksum(data []byte) ([]byte, error) {
	return bcrypt.GenerateFromPassword(data, bcrypt.MinCost)
}

// hashed password with its possible plaintext equivalent, return true if they match
func validateChecksum(checksum, data []byte) bool {
	err := bcrypt.CompareHashAndPassword(checksum, data)
	return err == nil
}

func encodeChecksum(data []byte) string {
	return hex.EncodeToString(data)
}

func decodeChecksum(s string) ([]byte, error) {
	return hex.DecodeString(s)
}
