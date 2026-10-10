package accounts

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
)

func NewCredential() (string, error) {
	var data [CredentialBytes]byte
	if _, err := rand.Read(data[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(data[:]), nil
}

func CredentialDigest(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

func ValidatePermissions(scopes []string) error {
	if len(scopes) == 0 || len(scopes) > 2 {
		return errors.New("invalid_permissions")
	}
	seen := make(map[string]bool)
	for _, scope := range scopes {
		if (scope != Read && scope != Ingest) || seen[scope] {
			return errors.New("invalid_permissions")
		}
		seen[scope] = true
	}
	return nil
}
