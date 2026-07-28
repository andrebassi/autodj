package app

import (
	"context"
	"errors"
	"testing"

	"autodj/domain/identity"
	"autodj/ports/inbound"
)

// fakeAuth records the last method invoked so the decorator's delegation can be
// asserted. Its returns are fixed sentinels distinct from the operator, so a test
// can tell "delegated" from "resolved to operator".
type fakeAuth struct {
	called   string
	user     identity.User
	token    string
	tokenArg string // last token seen by Me/Authorize
	meErr    error  // if set, Me returns it (simulates a stale/invalid token)
	authErr  error  // if set, Authorize returns it
}

func (f *fakeAuth) Login(_ context.Context, name, _ string) (string, identity.User, error) {
	f.called = "Login"
	return f.token, f.user, nil
}
func (f *fakeAuth) Authorize(_ context.Context, token string, _ identity.Permission) (identity.User, error) {
	f.called = "Authorize"
	f.tokenArg = token
	if f.authErr != nil {
		return identity.User{}, f.authErr
	}
	return f.user, nil
}
func (f *fakeAuth) Me(_ context.Context, token string) (identity.User, error) {
	f.called = "Me"
	f.tokenArg = token
	if f.meErr != nil {
		return identity.User{}, f.meErr
	}
	return f.user, nil
}
func (f *fakeAuth) ListUsers(context.Context, identity.User) ([]identity.User, error) {
	f.called = "ListUsers"
	return []identity.User{f.user}, nil
}
func (f *fakeAuth) CreateUser(_ context.Context, _ identity.User, name string, role identity.Role, _ string) (identity.User, error) {
	f.called = "CreateUser"
	return f.user, nil
}
func (f *fakeAuth) UpdateUser(_ context.Context, _ identity.User, id string, role identity.Role, _ string) (identity.User, error) {
	f.called = "UpdateUser"
	return f.user, nil
}
func (f *fakeAuth) DeleteUser(context.Context, identity.User, string) error {
	f.called = "DeleteUser"
	return nil
}

var _ inbound.AuthService = (*fakeAuth)(nil)

func mustUser(t *testing.T, name string, role identity.Role) identity.User {
	t.Helper()
	u, err := identity.NewUser("usr_"+name, name, role, "hash")
	if err != nil {
		t.Fatalf("NewUser: %v", err)
	}
	return u
}

func TestSingleUserMeEmptyTokenReturnsOperator(t *testing.T) {
	op := mustUser(t, "estacao", identity.RoleAdmin)
	inner := &fakeAuth{user: mustUser(t, "other", identity.RoleOperator)}
	s := NewSingleUserAuth(inner, op)

	got, err := s.Me(context.Background(), "")
	if err != nil {
		t.Fatalf("Me: %v", err)
	}
	if got.ID != op.ID {
		t.Fatalf("Me empty token = %q, want operator %q", got.ID, op.ID)
	}
	if inner.called != "" {
		t.Fatalf("inner should not be called, got %q", inner.called)
	}
}

func TestSingleUserMeWithTokenDelegates(t *testing.T) {
	op := mustUser(t, "estacao", identity.RoleAdmin)
	inner := &fakeAuth{user: mustUser(t, "other", identity.RoleOperator)}
	s := NewSingleUserAuth(inner, op)

	got, err := s.Me(context.Background(), "tok-123")
	if err != nil {
		t.Fatalf("Me: %v", err)
	}
	if inner.called != "Me" || inner.tokenArg != "tok-123" {
		t.Fatalf("delegate miss: called=%q token=%q", inner.called, inner.tokenArg)
	}
	if got.ID != inner.user.ID {
		t.Fatalf("Me token = %q, want inner %q", got.ID, inner.user.ID)
	}
}

func TestSingleUserMeStaleTokenFallsBackToOperator(t *testing.T) {
	// A leftover/invalid token in the client must NOT 401 in single-user — it
	// falls back to the operator so no login screen ever appears.
	op := mustUser(t, "estacao", identity.RoleAdmin)
	inner := &fakeAuth{user: mustUser(t, "other", identity.RoleOperator), meErr: inbound.ErrBadToken}
	s := NewSingleUserAuth(inner, op)

	got, err := s.Me(context.Background(), "stale-token")
	if err != nil {
		t.Fatalf("Me stale: %v", err)
	}
	if got.ID != op.ID {
		t.Fatalf("Me stale = %q, want operator %q", got.ID, op.ID)
	}
	if inner.called != "Me" {
		t.Fatalf("inner should be tried, got %q", inner.called)
	}
}

func TestSingleUserAuthorizeStaleTokenFallsBackToOperator(t *testing.T) {
	op := mustUser(t, "estacao", identity.RoleAdmin)
	inner := &fakeAuth{authErr: inbound.ErrBadToken}
	s := NewSingleUserAuth(inner, op)

	got, err := s.Authorize(context.Background(), "stale", identity.PermUsersManage)
	if err != nil {
		t.Fatalf("Authorize stale: %v", err)
	}
	if got.ID != op.ID {
		t.Fatalf("Authorize stale = %q, want operator %q", got.ID, op.ID)
	}
}

func TestSingleUserAuthorizeEmptyTokenAdminPasses(t *testing.T) {
	op := mustUser(t, "estacao", identity.RoleAdmin)
	inner := &fakeAuth{}
	s := NewSingleUserAuth(inner, op)

	got, err := s.Authorize(context.Background(), "", identity.PermUsersManage)
	if err != nil {
		t.Fatalf("Authorize admin: %v", err)
	}
	if got.ID != op.ID {
		t.Fatalf("Authorize = %q, want operator %q", got.ID, op.ID)
	}
	if inner.called != "" {
		t.Fatalf("inner should not be called, got %q", inner.called)
	}
}

func TestSingleUserAuthorizeEmptyTokenForbiddenWhenLacking(t *testing.T) {
	// A non-admin operator lacks users.manage → forbidden even with no token.
	op := mustUser(t, "estacao", identity.RoleOperator)
	inner := &fakeAuth{}
	s := NewSingleUserAuth(inner, op)

	_, err := s.Authorize(context.Background(), "", identity.PermUsersManage)
	if !errors.Is(err, inbound.ErrForbidden) {
		t.Fatalf("Authorize = %v, want ErrForbidden", err)
	}
}

func TestSingleUserAuthorizeWithTokenDelegates(t *testing.T) {
	op := mustUser(t, "estacao", identity.RoleAdmin)
	inner := &fakeAuth{user: mustUser(t, "other", identity.RoleOperator)}
	s := NewSingleUserAuth(inner, op)

	if _, err := s.Authorize(context.Background(), "tok", identity.PermPlaybackControl); err != nil {
		t.Fatalf("Authorize: %v", err)
	}
	if inner.called != "Authorize" || inner.tokenArg != "tok" {
		t.Fatalf("delegate miss: called=%q token=%q", inner.called, inner.tokenArg)
	}
}

func TestSingleUserDelegatesEverythingElse(t *testing.T) {
	op := mustUser(t, "estacao", identity.RoleAdmin)
	ctx := context.Background()

	cases := []struct {
		name string
		call func(s *SingleUserAuth, inner *fakeAuth)
		want string
	}{
		{"Login", func(s *SingleUserAuth, _ *fakeAuth) { _, _, _ = s.Login(ctx, "n", "p") }, "Login"},
		{"ListUsers", func(s *SingleUserAuth, _ *fakeAuth) { _, _ = s.ListUsers(ctx, op) }, "ListUsers"},
		{"CreateUser", func(s *SingleUserAuth, _ *fakeAuth) {
			_, _ = s.CreateUser(ctx, op, "n", identity.RoleOperator, "p")
		}, "CreateUser"},
		{"UpdateUser", func(s *SingleUserAuth, _ *fakeAuth) {
			_, _ = s.UpdateUser(ctx, op, "id", identity.RoleOperator, "p")
		}, "UpdateUser"},
		{"DeleteUser", func(s *SingleUserAuth, _ *fakeAuth) { _ = s.DeleteUser(ctx, op, "id") }, "DeleteUser"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			inner := &fakeAuth{user: op}
			s := NewSingleUserAuth(inner, op)
			tc.call(s, inner)
			if inner.called != tc.want {
				t.Fatalf("called %q, want %q", inner.called, tc.want)
			}
		})
	}
}
