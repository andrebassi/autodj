package app

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"strings"

	"autodj/domain/identity"
	"autodj/ports/inbound"
	"autodj/ports/outbound"
)

// AuthService authenticates users and gates access by the fixed role→permission
// matrix (Story 7.3). It composes three driven ports: a UserRepo (accounts), a
// PasswordHasher (argon2id) and a TokenIssuer (JWT). The domain (identity.Can)
// owns every access decision; this service only orchestrates I/O around it.
//
// Passwords are hashed on the way in and stored only as opaque hashes; Login is
// the sole place a plaintext password is handled, and it is never logged.
type AuthService struct {
	repo   outbound.UserRepo
	hasher outbound.PasswordHasher
	tokens outbound.TokenIssuer
}

// NewAuthService wires the service over its three ports.
func NewAuthService(repo outbound.UserRepo, hasher outbound.PasswordHasher, tokens outbound.TokenIssuer) *AuthService {
	return &AuthService{repo: repo, hasher: hasher, tokens: tokens}
}

// userID derives a stable id from the (unique) user name — like the library
// service ids tracks by path hash. Names are unique (CreateUser enforces it), so
// ids are unique too.
func userID(name string) string {
	sum := sha1.Sum([]byte(strings.ToLower(strings.TrimSpace(name))))
	return "usr_" + hex.EncodeToString(sum[:])[:12]
}

// SeedAdmin creates a default admin account when the repo is empty (first boot).
// The credential comes from the caller (env), never hardcoded. A no-op when any
// account already exists. Returns the created user, or a zero user when seeding
// was skipped.
func (s *AuthService) SeedAdmin(ctx context.Context, name, password string) (identity.User, error) {
	n, err := s.repo.Count(ctx)
	if err != nil {
		return identity.User{}, err
	}
	if n > 0 {
		return identity.User{}, nil
	}
	hash, err := s.hasher.Hash(password)
	if err != nil {
		return identity.User{}, err
	}
	u, err := identity.NewUser(userID(name), name, identity.RoleAdmin, hash)
	if err != nil {
		return identity.User{}, err
	}
	if err := s.repo.Upsert(ctx, u); err != nil {
		return identity.User{}, err
	}
	return u, nil
}

// Login verifies name+password and issues a token. An unknown name and a wrong
// password both return ErrBadCredentials — indistinguishable on purpose.
func (s *AuthService) Login(ctx context.Context, name, password string) (string, identity.User, error) {
	u, err := s.repo.GetByName(ctx, name)
	if err != nil {
		if errors.Is(err, outbound.ErrUserNotFound) {
			return "", identity.User{}, inbound.ErrBadCredentials
		}
		return "", identity.User{}, err
	}
	if !s.hasher.Verify(password, u.PassHash) {
		return "", identity.User{}, inbound.ErrBadCredentials
	}
	token, err := s.tokens.Issue(u)
	if err != nil {
		return "", identity.User{}, err
	}
	return token, u, nil
}

// Me parses the token and loads the current user. Returns ErrBadToken for a
// missing/invalid token or ErrUserNotFound when the account no longer exists.
func (s *AuthService) Me(ctx context.Context, token string) (identity.User, error) {
	claims, err := s.tokens.Parse(token)
	if err != nil {
		return identity.User{}, inbound.ErrBadToken
	}
	u, err := s.repo.Get(ctx, claims.UserID)
	if err != nil {
		if errors.Is(err, outbound.ErrUserNotFound) {
			return identity.User{}, inbound.ErrUserNotFound
		}
		return identity.User{}, err
	}
	return u, nil
}

// Authorize parses the token, loads the user and checks perm against its role.
// Returns ErrBadToken for a bad token or ErrForbidden when the role lacks the
// permission.
func (s *AuthService) Authorize(ctx context.Context, token string, perm identity.Permission) (identity.User, error) {
	u, err := s.Me(ctx, token)
	if err != nil {
		return identity.User{}, err
	}
	if !u.Can(perm) {
		return identity.User{}, inbound.ErrForbidden
	}
	return u, nil
}

// ListUsers returns every account. Requires the actor to hold users.manage.
func (s *AuthService) ListUsers(ctx context.Context, actor identity.User) ([]identity.User, error) {
	if !actor.Can(identity.PermUsersManage) {
		return nil, inbound.ErrForbidden
	}
	return s.repo.List(ctx)
}

// CreateUser validates and stores a new account with the given plaintext
// password. Requires users.manage. Returns ErrForbidden, an identity validation
// error, or ErrNameTaken when the name is taken.
func (s *AuthService) CreateUser(ctx context.Context, actor identity.User, name string, role identity.Role, password string) (identity.User, error) {
	if !actor.Can(identity.PermUsersManage) {
		return identity.User{}, inbound.ErrForbidden
	}
	// validate name/role before hashing so a bad request never hashes.
	if _, err := identity.NewUser(userID(name), name, role, "x"); err != nil {
		return identity.User{}, err
	}
	if _, err := s.repo.GetByName(ctx, name); err == nil {
		return identity.User{}, inbound.ErrNameTaken
	} else if !errors.Is(err, outbound.ErrUserNotFound) {
		return identity.User{}, err
	}
	hash, err := s.hasher.Hash(password)
	if err != nil {
		return identity.User{}, err
	}
	u, err := identity.NewUser(userID(name), name, role, hash)
	if err != nil {
		return identity.User{}, err
	}
	if err := s.repo.Upsert(ctx, u); err != nil {
		return identity.User{}, err
	}
	return u, nil
}

// UpdateUser changes a user's role and, when password is non-empty, its password.
// Requires users.manage. Returns ErrForbidden, ErrUserNotFound or a validation error.
func (s *AuthService) UpdateUser(ctx context.Context, actor identity.User, id string, role identity.Role, password string) (identity.User, error) {
	if !actor.Can(identity.PermUsersManage) {
		return identity.User{}, inbound.ErrForbidden
	}
	cur, err := s.repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, outbound.ErrUserNotFound) {
			return identity.User{}, inbound.ErrUserNotFound
		}
		return identity.User{}, err
	}
	hash := cur.PassHash
	if password != "" {
		hash, err = s.hasher.Hash(password)
		if err != nil {
			return identity.User{}, err
		}
	}
	u, err := identity.NewUser(cur.ID, cur.Name, role, hash)
	if err != nil {
		return identity.User{}, err
	}
	if err := s.repo.Upsert(ctx, u); err != nil {
		return identity.User{}, err
	}
	return u, nil
}

// DeleteUser removes a user by id. Requires users.manage. Returns ErrForbidden or
// ErrUserNotFound.
func (s *AuthService) DeleteUser(ctx context.Context, actor identity.User, id string) error {
	if !actor.Can(identity.PermUsersManage) {
		return inbound.ErrForbidden
	}
	if err := s.repo.Delete(ctx, id); err != nil {
		if errors.Is(err, outbound.ErrUserNotFound) {
			return inbound.ErrUserNotFound
		}
		return err
	}
	return nil
}

// compile-time: AuthService satisfies the inbound port.
var _ inbound.AuthService = (*AuthService)(nil)
