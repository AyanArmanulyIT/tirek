// Package identity implements the Tirek identity module: users, authentication
// (argon2id passwords, JWT access tokens, rotating refresh tokens with reuse
// detection), sessions, and the auth HTTP handlers.
package identity

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Argon2id parameters (OWASP recommended interactive settings).
const (
	argon2Memory      = 19 * 1024 // 19 MiB
	argon2Iterations  = 2
	argon2Parallelism = 1
	argon2SaltLen     = 16
	argon2KeyLen      = 32
	argon2Version     = 0x13
)

// PasswordHasher hashes and verifies passwords using Argon2id, encoded in the
// PHC string format: $argon2id$v=19$m=...,t=...,p=...$<salt>$<hash>.
type PasswordHasher struct{}

// Hash encodes a password as a PHC argon2id string. Empty passwords are
// rejected (callers validate policy; this is defense in depth).
func (PasswordHasher) Hash(password string) (string, error) {
	if strings.TrimSpace(password) == "" {
		return "", errors.New("password must not be empty")
	}
	salt := make([]byte, argon2SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}
	hash := argon2.IDKey([]byte(password), salt, argon2Iterations, argon2Memory, argon2Parallelism, argon2KeyLen)
	return encodePHC(salt, hash), nil
}

// Verify checks password against an encoded PHC argon2id string.
func (PasswordHasher) Verify(password, encoded string) (bool, error) {
	p, err := decodePHC(encoded)
	if err != nil {
		return false, err
	}
	derived := argon2.IDKey([]byte(password), p.salt, p.iterations, p.memory, p.parallelism, argon2KeyLen)
	if subtle.ConstantTimeCompare(derived, p.hash) != 1 {
		return false, nil
	}
	return true, nil
}

type phcParams struct {
	memory      uint32
	iterations  uint32
	parallelism uint8
	salt        []byte
	hash        []byte
}

func encodePHC(salt, hash []byte) string {
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2Version,
		argon2Memory, argon2Iterations, argon2Parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash))
}

func decodePHC(encoded string) (phcParams, error) {
	var p phcParams
	parts := strings.Split(encoded, "$")
	// ["", "argon2id", "v=19", "m=65536,t=3,p=2", salt, hash]
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return p, errors.New("malformed password hash")
	}
	if parts[2] != "v=19" {
		return p, errors.New("unsupported argon2 version")
	}
	var m, t, pn int
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &pn); err != nil {
		return p, errors.New("malformed argon2 parameters")
	}
	if m <= 0 || t <= 0 || pn <= 0 {
		return p, errors.New("invalid argon2 parameters")
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) == 0 {
		return p, errors.New("malformed argon2 salt")
	}
	hash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(hash) == 0 {
		return p, errors.New("malformed argon2 hash")
	}
	p.memory = uint32(m)
	p.iterations = uint32(t)
	p.parallelism = uint8(pn)
	p.salt = salt
	p.hash = hash
	return p, nil
}