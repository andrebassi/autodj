package app

import (
	"context"

	"autodj/domain/identity"
	"autodj/ports/inbound"
)

// SingleUserAuth adapts an AuthService for home 1:1 use: one operator, no login
// (Story 9.4). It NEVER shows a login screen: Me and Authorize always resolve to a
// fixed operator. With no bearer token — every request from the home UI — that is
// immediate. With a token present, it delegates to the inner service and, only if
// that token is still valid, honours the delegated identity; a stale or invalid
// token (e.g. left in localStorage from a past multi-user session) does NOT 401 —
// it falls back to the operator, so single-user stays truly 1:1 regardless of
// leftover client state. Login and the user-management CRUD delegate untouched, so
// RBAC and account management keep working against the real repo. Flipping back to
// multi-user is env-only (AUTODJ_SINGLE_USER=0 skips installing this decorator).
//
// The operator is the seeded admin (all permissions), so Authorize passes every
// permission without a per-permission special case; a non-admin operator would
// still be gated by identity.Can.
type SingleUserAuth struct {
	inner    inbound.AuthService
	operator identity.User
}

// NewSingleUserAuth wraps inner, resolving token-less requests to operator.
func NewSingleUserAuth(inner inbound.AuthService, operator identity.User) *SingleUserAuth {
	return &SingleUserAuth{inner: inner, operator: operator}
}

// Me returns the operator for a token-less request. With a token it delegates,
// but a stale/invalid token falls back to the operator — single-user never 401s.
func (s *SingleUserAuth) Me(ctx context.Context, token string) (identity.User, error) {
	if token == "" {
		return s.operator, nil
	}
	if u, err := s.inner.Me(ctx, token); err == nil {
		return u, nil
	}
	return s.operator, nil
}

// Authorize authorizes by the domain matrix. Token-less resolves to the operator;
// with a token it delegates, and only a valid token overrides the operator — a
// stale/invalid token falls back to the operator (never a login screen). Either
// way ErrForbidden when the resolved identity lacks perm.
func (s *SingleUserAuth) Authorize(ctx context.Context, token string, perm identity.Permission) (identity.User, error) {
	if token != "" {
		if u, err := s.inner.Authorize(ctx, token, perm); err == nil {
			return u, nil
		}
		// stale/invalid token in single-user: fall through to the operator.
	}
	if !s.operator.Can(perm) {
		return identity.User{}, inbound.ErrForbidden
	}
	return s.operator, nil
}

// Login delegates unchanged (unused by the home UI, but kept working).
func (s *SingleUserAuth) Login(ctx context.Context, name, password string) (string, identity.User, error) {
	return s.inner.Login(ctx, name, password)
}

// ListUsers delegates unchanged.
func (s *SingleUserAuth) ListUsers(ctx context.Context, actor identity.User) ([]identity.User, error) {
	return s.inner.ListUsers(ctx, actor)
}

// CreateUser delegates unchanged.
func (s *SingleUserAuth) CreateUser(ctx context.Context, actor identity.User, name string, role identity.Role, password string) (identity.User, error) {
	return s.inner.CreateUser(ctx, actor, name, role, password)
}

// UpdateUser delegates unchanged.
func (s *SingleUserAuth) UpdateUser(ctx context.Context, actor identity.User, id string, role identity.Role, password string) (identity.User, error) {
	return s.inner.UpdateUser(ctx, actor, id, role, password)
}

// DeleteUser delegates unchanged.
func (s *SingleUserAuth) DeleteUser(ctx context.Context, actor identity.User, id string) error {
	return s.inner.DeleteUser(ctx, actor, id)
}

// compile-time: SingleUserAuth satisfies the inbound port.
var _ inbound.AuthService = (*SingleUserAuth)(nil)
