package auth

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Password hashes use PBKDF2-HMAC-SHA256 from Go's standard library, so no
// extra dependency is needed. Format (no "$", so docker compose and shells
// paste it into .env without any quoting):
//
//	pbkdf2-sha256:600000:<salt, base64>:<key, base64>
//
// Create one with:  natlas hash-password
const (
	hashPrefix     = "pbkdf2-sha256"
	hashIterations = 600_000 // OWASP 2023 recommendation for PBKDF2-HMAC-SHA256
	hashKeyLen     = 32
)

// HashPassword returns a new salted hash for password.
func HashPassword(password string) (string, error) {
	if password == "" {
		return "", errors.New("password is empty")
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key, err := pbkdf2.Key(sha256.New, password, salt, hashIterations, hashKeyLen)
	if err != nil {
		return "", err
	}
	enc := base64.RawStdEncoding
	return fmt.Sprintf("%s:%d:%s:%s", hashPrefix, hashIterations, enc.EncodeToString(salt), enc.EncodeToString(key)), nil
}

// ValidHash reports whether s looks like a hash made by HashPassword.
func ValidHash(s string) error {
	_, _, _, err := parseHash(s)
	return err
}

func parseHash(s string) (iter int, salt, key []byte, err error) {
	parts := strings.Split(strings.TrimSpace(s), ":")
	if len(parts) != 4 || parts[0] != hashPrefix {
		return 0, nil, nil, errors.New("not a natlas password hash (expected pbkdf2-sha256:…; create one with: natlas hash-password)")
	}
	iter, err = strconv.Atoi(parts[1])
	if err != nil || iter < 10_000 {
		return 0, nil, nil, errors.New("password hash: bad iteration count")
	}
	enc := base64.RawStdEncoding
	if salt, err = enc.DecodeString(parts[2]); err != nil || len(salt) < 8 {
		return 0, nil, nil, errors.New("password hash: bad salt")
	}
	if key, err = enc.DecodeString(parts[3]); err != nil || len(key) < 16 {
		return 0, nil, nil, errors.New("password hash: bad key")
	}
	return iter, salt, key, nil
}

// checkHash verifies password against a hash in constant time.
func checkHash(hash, password string) bool {
	iter, salt, want, err := parseHash(hash)
	if err != nil {
		return false
	}
	got, err := pbkdf2.Key(sha256.New, password, salt, iter, len(want))
	return err == nil && subtle.ConstantTimeCompare(got, want) == 1
}
