// Package argon2id implements the outbound.PasswordHasher port using argon2id
// (the password-hashing-competition winner, memory-hard, side-channel resistant).
// Hashes are self-describing PHC strings — salt and parameters travel with the
// hash — so Verify needs no external config. Plaintext passwords are never logged.
package argon2id

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"

	"autodj/ports/outbound"
)

// Params tunes the argon2id cost. Defaults follow current OWASP guidance for
// interactive logins (64 MiB, 1 pass, 4 lanes) — comfortable on a home machine.
type Params struct {
	Memory  uint32 // KiB
	Time    uint32 // passes
	Threads uint8  // lanes
	KeyLen  uint32 // derived key length in bytes
	SaltLen uint32 // random salt length in bytes
}

// DefaultParams is the recommended interactive-login cost.
var DefaultParams = Params{
	Memory:  64 * 1024,
	Time:    1,
	Threads: 4,
	KeyLen:  32,
	SaltLen: 16,
}

// Hasher hashes and verifies passwords with a fixed parameter set.
type Hasher struct {
	p Params
}

// New builds a hasher with the default parameters.
func New() *Hasher { return &Hasher{p: DefaultParams} }

// NewWith builds a hasher with custom parameters (used by tests to keep cost low).
func NewWith(p Params) *Hasher { return &Hasher{p: p} }

// Hash derives a PHC-formatted argon2id hash:
//
//	$argon2id$v=19$m=<mem>,t=<time>,p=<threads>$<b64salt>$<b64hash>
func (h *Hasher) Hash(password string) (string, error) {
	salt := make([]byte, h.p.SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(password), salt, h.p.Time, h.p.Memory, h.p.Threads, h.p.KeyLen)
	return fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		h.p.Memory, h.p.Time, h.p.Threads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	), nil
}

// Verify reports whether password matches the PHC hash in constant time. Any
// malformed hash simply returns false (never panics, never leaks the reason).
func (h *Hasher) Verify(password, hash string) bool {
	p, salt, want, err := decode(hash)
	if err != nil {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, p.Time, p.Memory, p.Threads, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}

// errBadHash marks an unparseable PHC string.
var errBadHash = errors.New("argon2id: malformed hash")

// decode parses a PHC argon2id string back into its params, salt and key.
func decode(hash string) (Params, []byte, []byte, error) {
	parts := strings.Split(hash, "$")
	// ["", "argon2id", "v=19", "m=..,t=..,p=..", "<salt>", "<key>"]
	if len(parts) != 6 || parts[1] != "argon2id" {
		return Params{}, nil, nil, errBadHash
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return Params{}, nil, nil, errBadHash
	}
	var p Params
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.Memory, &p.Time, &p.Threads); err != nil {
		return Params{}, nil, nil, errBadHash
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return Params{}, nil, nil, errBadHash
	}
	key, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return Params{}, nil, nil, errBadHash
	}
	return p, salt, key, nil
}

// compile-time: Hasher satisfies the outbound port.
var _ outbound.PasswordHasher = (*Hasher)(nil)
