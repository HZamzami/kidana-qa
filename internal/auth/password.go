// Package auth handles password hashing and login throttling.
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// argon2id parameters (OWASP recommendation: m=19 MiB, t=2, p=1).
const (
	argonMemory  = 19 * 1024
	argonTime    = 2
	argonThreads = 1
	argonKeyLen  = 32
	saltLen      = 16
	// MinPasswordLength is enforced when users set passwords.
	MinPasswordLength = 10
)

var errBadHash = errors.New("invalid password hash format")

// HashPassword returns an encoded argon2id hash in the PHC string format.
func HashPassword(password string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key)), nil
}

// CheckPassword reports whether password matches the encoded hash.
func CheckPassword(encoded, password string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, errBadHash
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return false, errBadHash
	}
	var m uint32
	var t uint32
	var p uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil {
		return false, errBadHash
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, errBadHash
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, errBadHash
	}
	got := argon2.IDKey([]byte(password), salt, t, m, p, uint32(len(want))) //nolint:gosec // len of a 32-byte key
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

// dummyHash is compared against when the user does not exist, so login takes
// the same time whether or not the email is registered.
var dummyHash, _ = HashPassword("dummy-password-for-timing")

// CheckPasswordOrDummy runs a full hash comparison even when there is no user.
func CheckPasswordOrDummy(encoded *string, password string) bool {
	if encoded == nil {
		_, _ = CheckPassword(dummyHash, password)
		return false
	}
	ok, err := CheckPassword(*encoded, password)
	return err == nil && ok
}
