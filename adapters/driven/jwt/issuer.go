// Package jwt implements the outbound.TokenIssuer port with a minimal, self-
// contained HS256 JSON Web Token — no external dependency. Tokens carry the user
// id (sub) and role, and an expiry (exp); Parse validates the HMAC signature and
// the expiry in constant time. The signing secret comes from the caller (env),
// never hardcoded.
package jwt

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"autodj/domain/identity"
	"autodj/ports/outbound"
)

// Errors returned by Parse.
var (
	// ErrMalformed marks a token that is not three base64url segments or whose
	// header/payload does not decode.
	ErrMalformed = errors.New("jwt: malformed token")
	// ErrSignature marks a token whose HMAC does not match (tampered/wrong key).
	ErrSignature = errors.New("jwt: bad signature")
	// ErrExpired marks a token past its exp.
	ErrExpired = errors.New("jwt: expired")
)

// header is the fixed HS256 JWT header.
type header struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
}

// payload is the token's claim set.
type payload struct {
	Sub  string        `json:"sub"`
	Role identity.Role `json:"role"`
	Exp  int64         `json:"exp"`
}

// Issuer signs and parses tokens with one secret, TTL and clock.
type Issuer struct {
	secret []byte
	ttl    time.Duration
	now    func() time.Time
}

// New builds an issuer over a signing secret and token lifetime (e.g. 8h).
func New(secret []byte, ttl time.Duration) *Issuer {
	return &Issuer{secret: secret, ttl: ttl, now: time.Now}
}

// Issue signs a token for the user, expiring ttl from now.
func (i *Issuer) Issue(u identity.User) (string, error) {
	h, err := json.Marshal(header{Alg: "HS256", Typ: "JWT"})
	if err != nil {
		return "", err
	}
	p, err := json.Marshal(payload{
		Sub:  u.ID,
		Role: u.Role,
		Exp:  i.now().Add(i.ttl).Unix(),
	})
	if err != nil {
		return "", err
	}
	signing := enc(h) + "." + enc(p)
	return signing + "." + enc(i.sign(signing)), nil
}

// Parse validates the signature and expiry and returns the claims.
func (i *Issuer) Parse(token string) (outbound.Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return outbound.Claims{}, ErrMalformed
	}
	signing := parts[0] + "." + parts[1]
	sig, err := dec(parts[2])
	if err != nil {
		return outbound.Claims{}, ErrMalformed
	}
	if !hmac.Equal(sig, i.sign(signing)) {
		return outbound.Claims{}, ErrSignature
	}
	raw, err := dec(parts[1])
	if err != nil {
		return outbound.Claims{}, ErrMalformed
	}
	var p payload
	if err := json.Unmarshal(raw, &p); err != nil {
		return outbound.Claims{}, ErrMalformed
	}
	if i.now().Unix() >= p.Exp {
		return outbound.Claims{}, ErrExpired
	}
	return outbound.Claims{UserID: p.Sub, Role: p.Role}, nil
}

// sign computes the HMAC-SHA256 of the signing input.
func (i *Issuer) sign(signing string) []byte {
	m := hmac.New(sha256.New, i.secret)
	m.Write([]byte(signing))
	return m.Sum(nil)
}

// enc/dec are base64url without padding, per the JWT spec.
func enc(b []byte) string          { return base64.RawURLEncoding.EncodeToString(b) }
func dec(s string) ([]byte, error) { return base64.RawURLEncoding.DecodeString(s) }

// compile-time: Issuer satisfies the outbound port.
var _ outbound.TokenIssuer = (*Issuer)(nil)
