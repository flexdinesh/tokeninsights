package accounts

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

const (
	MinPasswordCharacters = 15
	MaxPasswordCharacters = 128
	MinUsernameBytes      = 3
	MaxUsernameBytes      = 32
	passwordMemory        = 19 * 1024
	passwordIterations    = 2
	passwordParallelism   = 1
	passwordSaltBytes     = 16
	passwordHashBytes     = 32
)

var (
	ErrCurrentPasswordMismatch = errors.New("current_password_incorrect")
	ErrInvalidUsername         = errors.New("invalid_username")
	ErrInvalidPassword         = errors.New("invalid_password")
	ErrUsernameTaken           = errors.New("username_taken")
	ErrInvalidLabel            = errors.New("invalid_key_label")
	ErrInvalidPermissions      = errors.New("invalid_permissions")
	ErrInvalidExpiry           = errors.New("invalid_expiry")
)

func NormalizeUsername(username string) (string, error) {
	username = strings.TrimSpace(username)
	if len(username) < MinUsernameBytes || len(username) > MaxUsernameBytes {
		return "", ErrInvalidUsername
	}
	for i, c := range username {
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || i > 0 && (c == '_' || c == '-') {
			continue
		}
		return "", ErrInvalidUsername
	}
	return strings.ToLower(username), nil
}
func ValidatePassword(password string) error {
	if !utf8.ValidString(password) {
		return ErrInvalidPassword
	}
	count := utf8.RuneCountInString(password)
	if count < MinPasswordCharacters || count > MaxPasswordCharacters {
		return ErrInvalidPassword
	}
	return nil
}
func ValidateBootstrap(username, password string) error {
	if _, err := NormalizeUsername(username); err != nil {
		return err
	}
	return ValidatePassword(password)
}
func HashPassword(password string) (string, error) {
	if err := ValidatePassword(password); err != nil {
		return "", err
	}
	salt := make([]byte, passwordSaltBytes)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	hash := argon2.IDKey([]byte(password), salt, passwordIterations, passwordMemory, passwordParallelism, passwordHashBytes)
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s", passwordMemory, passwordIterations, passwordParallelism, base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(hash)), nil
}

// VerifyPassword accepts only our bounded current hash policy, so corrupted
// persistence cannot request unbounded memory or CPU during authentication.
func VerifyPassword(password, encoded string) bool {
	if ValidatePassword(password) != nil {
		return false
	}
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v=19" || parts[3] != fmt.Sprintf("m=%d,t=%d,p=%d", passwordMemory, passwordIterations, passwordParallelism) {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) != passwordSaltBytes {
		return false
	}
	hash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(hash) != passwordHashBytes {
		return false
	}
	actual := argon2.IDKey([]byte(password), salt, passwordIterations, passwordMemory, passwordParallelism, passwordHashBytes)
	return subtle.ConstantTimeCompare(actual, hash) == 1
}

// DummyPasswordHash has the same cost as a stored password and prevents missing
// usernames from avoiding the password check. Its credential is never accepted.
const DummyPasswordHash = "$argon2id$v=19$m=19456,t=2,p=1$AAAAAAAAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
