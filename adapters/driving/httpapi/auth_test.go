package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"autodj/domain/identity"
	"autodj/ports/inbound"
)

// fakeAuth is a configurable AuthService stub. authOK gates Authorize (mimics a
// user holding the permission); when authErr is set Authorize returns it.
type fakeAuth struct {
	loginErr  error
	loginTok  string
	loginUser identity.User

	meErr  error
	meUser identity.User

	authErr  error
	authUser identity.User

	listErr   error
	listUsers []identity.User

	createErr  error
	createUser identity.User
	gotName    string
	gotRole    identity.Role
	gotPass    string

	updateErr  error
	updateUser identity.User
	updID      string

	deleteErr error
	delID     string
}

func (f *fakeAuth) Login(_ context.Context, name, password string) (string, identity.User, error) {
	if f.loginErr != nil {
		return "", identity.User{}, f.loginErr
	}
	return f.loginTok, f.loginUser, nil
}
func (f *fakeAuth) Authorize(_ context.Context, _ string, _ identity.Permission) (identity.User, error) {
	if f.authErr != nil {
		return identity.User{}, f.authErr
	}
	return f.authUser, nil
}
func (f *fakeAuth) Me(_ context.Context, _ string) (identity.User, error) {
	if f.meErr != nil {
		return identity.User{}, f.meErr
	}
	return f.meUser, nil
}
func (f *fakeAuth) ListUsers(context.Context, identity.User) ([]identity.User, error) {
	return f.listUsers, f.listErr
}
func (f *fakeAuth) CreateUser(_ context.Context, _ identity.User, name string, role identity.Role, password string) (identity.User, error) {
	f.gotName, f.gotRole, f.gotPass = name, role, password
	if f.createErr != nil {
		return identity.User{}, f.createErr
	}
	return f.createUser, nil
}
func (f *fakeAuth) UpdateUser(_ context.Context, _ identity.User, id string, _ identity.Role, _ string) (identity.User, error) {
	f.updID = id
	if f.updateErr != nil {
		return identity.User{}, f.updateErr
	}
	return f.updateUser, nil
}
func (f *fakeAuth) DeleteUser(_ context.Context, _ identity.User, id string) error {
	f.delID = id
	return f.deleteErr
}

func mkAdmin() identity.User {
	u, _ := identity.NewUser("usr_admin", "root", identity.RoleAdmin, "h")
	return u
}

// doAuth runs a request carrying a bearer token.
func doAuth(t *testing.T, h http.Handler, method, path, body, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func withAuth(svc inbound.AuthService) http.Handler {
	return New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil, WithAuth(svc))
}

func TestLoginOK(t *testing.T) {
	svc := &fakeAuth{loginTok: "T:abc", loginUser: mkAdmin()}
	h := withAuth(svc)
	rec := do(t, h, http.MethodPost, "/api/v1/auth/login", `{"name":"root","password":"pw"}`)
	if rec.Code != 200 {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body)
	}
	var got struct {
		Token string  `json:"token"`
		User  userDTO `json:"user"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Token != "T:abc" || got.User.Name != "root" || got.User.Role != "admin" {
		t.Fatalf("got %+v", got)
	}
	// admin resolves the full permission set including users.manage
	if !contains(got.User.Permissions, string(identity.PermUsersManage)) {
		t.Fatalf("admin perms missing users.manage: %v", got.User.Permissions)
	}
}

func TestLoginBadCredentials(t *testing.T) {
	svc := &fakeAuth{loginErr: inbound.ErrBadCredentials}
	rec := do(t, withAuth(svc), http.MethodPost, "/api/v1/auth/login", `{"name":"x","password":"y"}`)
	if rec.Code != 401 {
		t.Fatalf("bad creds should 401, got %d", rec.Code)
	}
}

func TestLoginBadBody(t *testing.T) {
	svc := &fakeAuth{}
	rec := do(t, withAuth(svc), http.MethodPost, "/api/v1/auth/login", `{`)
	if rec.Code != 400 {
		t.Fatalf("malformed body should 400, got %d", rec.Code)
	}
}

func TestMeOK(t *testing.T) {
	svc := &fakeAuth{meUser: mkAdmin()}
	rec := doAuth(t, withAuth(svc), http.MethodGet, "/api/v1/auth/me", "", "T:abc")
	if rec.Code != 200 {
		t.Fatalf("code=%d", rec.Code)
	}
	var u userDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &u); err != nil {
		t.Fatal(err)
	}
	if u.Name != "root" {
		t.Fatalf("me=%+v", u)
	}
}

func TestMeBadToken(t *testing.T) {
	svc := &fakeAuth{meErr: inbound.ErrBadToken}
	rec := doAuth(t, withAuth(svc), http.MethodGet, "/api/v1/auth/me", "", "")
	if rec.Code != 401 {
		t.Fatalf("bad token should 401, got %d", rec.Code)
	}
}

func TestListUsersAuthorized(t *testing.T) {
	admin, _ := identity.NewUser("a", "Ana", identity.RoleAdmin, "h")
	op, _ := identity.NewUser("b", "Bob", identity.RoleOperator, "h")
	svc := &fakeAuth{authUser: mkAdmin(), listUsers: []identity.User{admin, op}}
	rec := doAuth(t, withAuth(svc), http.MethodGet, "/api/v1/users", "", "T:admin")
	if rec.Code != 200 {
		t.Fatalf("code=%d", rec.Code)
	}
	var got []userDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Name != "Ana" {
		t.Fatalf("list=%+v", got)
	}
}

func TestUsersForbidden(t *testing.T) {
	svc := &fakeAuth{authErr: inbound.ErrForbidden}
	rec := doAuth(t, withAuth(svc), http.MethodGet, "/api/v1/users", "", "T:op")
	if rec.Code != 403 {
		t.Fatalf("lacking perm should 403, got %d", rec.Code)
	}
}

func TestUsersUnauthorized(t *testing.T) {
	svc := &fakeAuth{authErr: inbound.ErrBadToken}
	rec := doAuth(t, withAuth(svc), http.MethodGet, "/api/v1/users", "", "")
	if rec.Code != 401 {
		t.Fatalf("no token should 401, got %d", rec.Code)
	}
}

func TestCreateUserOK(t *testing.T) {
	created, _ := identity.NewUser("usr_x", "Novo", identity.RoleOperator, "h")
	svc := &fakeAuth{authUser: mkAdmin(), createUser: created}
	rec := doAuth(t, withAuth(svc), http.MethodPost, "/api/v1/users",
		`{"name":"Novo","role":"operador","password":"pw"}`, "T:admin")
	if rec.Code != 201 {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body)
	}
	if svc.gotName != "Novo" || svc.gotRole != identity.RoleOperator || svc.gotPass != "pw" {
		t.Fatalf("service got name=%q role=%q pass=%q", svc.gotName, svc.gotRole, svc.gotPass)
	}
}

func TestCreateUserBadBody(t *testing.T) {
	svc := &fakeAuth{authUser: mkAdmin()}
	rec := doAuth(t, withAuth(svc), http.MethodPost, "/api/v1/users", `{`, "T:admin")
	if rec.Code != 400 {
		t.Fatalf("malformed body should 400, got %d", rec.Code)
	}
}

func TestCreateUserNameTaken(t *testing.T) {
	svc := &fakeAuth{authUser: mkAdmin(), createErr: inbound.ErrNameTaken}
	rec := doAuth(t, withAuth(svc), http.MethodPost, "/api/v1/users",
		`{"name":"root","role":"operador","password":"pw"}`, "T:admin")
	if rec.Code != 409 {
		t.Fatalf("dup name should 409, got %d", rec.Code)
	}
}

func TestCreateUserBadRole(t *testing.T) {
	svc := &fakeAuth{authUser: mkAdmin(), createErr: identity.ErrBadRole}
	rec := doAuth(t, withAuth(svc), http.MethodPost, "/api/v1/users",
		`{"name":"X","role":"ceo","password":"pw"}`, "T:admin")
	if rec.Code != 422 {
		t.Fatalf("bad role should 422, got %d", rec.Code)
	}
}

func TestUpdateUserOK(t *testing.T) {
	upd, _ := identity.NewUser("usr_x", "Novo", identity.RoleAnnouncer, "h")
	svc := &fakeAuth{authUser: mkAdmin(), updateUser: upd}
	rec := doAuth(t, withAuth(svc), http.MethodPatch, "/api/v1/users/usr_x",
		`{"role":"locutor"}`, "T:admin")
	if rec.Code != 200 {
		t.Fatalf("code=%d", rec.Code)
	}
	if svc.updID != "usr_x" {
		t.Fatalf("updID=%q", svc.updID)
	}
}

func TestUpdateUserNotFound(t *testing.T) {
	svc := &fakeAuth{authUser: mkAdmin(), updateErr: inbound.ErrUserNotFound}
	rec := doAuth(t, withAuth(svc), http.MethodPatch, "/api/v1/users/ghost",
		`{"role":"locutor"}`, "T:admin")
	if rec.Code != 404 {
		t.Fatalf("unknown id should 404, got %d", rec.Code)
	}
}

func TestUpdateUserBadBody(t *testing.T) {
	svc := &fakeAuth{authUser: mkAdmin()}
	rec := doAuth(t, withAuth(svc), http.MethodPatch, "/api/v1/users/usr_x", `{`, "T:admin")
	if rec.Code != 400 {
		t.Fatalf("malformed body should 400, got %d", rec.Code)
	}
}

func TestDeleteUserOK(t *testing.T) {
	svc := &fakeAuth{authUser: mkAdmin()}
	rec := doAuth(t, withAuth(svc), http.MethodDelete, "/api/v1/users/usr_x", "", "T:admin")
	if rec.Code != 204 {
		t.Fatalf("code=%d", rec.Code)
	}
	if svc.delID != "usr_x" {
		t.Fatalf("delID=%q", svc.delID)
	}
}

func TestDeleteUserNotFound(t *testing.T) {
	svc := &fakeAuth{authUser: mkAdmin(), deleteErr: inbound.ErrUserNotFound}
	rec := doAuth(t, withAuth(svc), http.MethodDelete, "/api/v1/users/ghost", "", "T:admin")
	if rec.Code != 404 {
		t.Fatalf("unknown id should 404, got %d", rec.Code)
	}
}

func TestAuthRoutesAbsentWithoutOption(t *testing.T) {
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil)
	for _, tc := range []struct{ method, path, body string }{
		{http.MethodPost, "/api/v1/auth/login", `{"name":"x","password":"y"}`},
		{http.MethodGet, "/api/v1/auth/me", ""},
		{http.MethodGet, "/api/v1/users", ""},
		{http.MethodPost, "/api/v1/users", `{}`},
		{http.MethodPatch, "/api/v1/users/x", `{}`},
		{http.MethodDelete, "/api/v1/users/x", ""},
	} {
		if rec := do(t, h, tc.method, tc.path, tc.body); rec.Code != 404 {
			t.Fatalf("%s %s should 404 without WithAuth, got %d", tc.method, tc.path, rec.Code)
		}
	}
}

// TestBearerParsing checks the Authorization header extraction edge cases via me.
func TestBearerParsing(t *testing.T) {
	svc := &fakeAuth{meUser: mkAdmin()}
	h := withAuth(svc)
	// lowercase scheme still parses (case-insensitive)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.Header.Set("Authorization", "bearer T:abc")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("lowercase bearer should parse, got %d", rec.Code)
	}
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}
