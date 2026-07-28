package app

import (
	"context"
	"errors"
	"testing"

	"autodj/domain/identity"
	"autodj/ports/inbound"
	"autodj/ports/outbound"
)

// --- fakes ---

// memRepo is an in-memory UserRepo keyed by id, with a name index.
type memRepo struct {
	byID   map[string]identity.User
	failOn string // when set, matching method returns errAuthBoom
}

var errAuthBoom = errors.New("boom")

func newMemRepo() *memRepo { return &memRepo{byID: map[string]identity.User{}} }

func (r *memRepo) Get(_ context.Context, id string) (identity.User, error) {
	if r.failOn == "Get" {
		return identity.User{}, errAuthBoom
	}
	u, ok := r.byID[id]
	if !ok {
		return identity.User{}, outbound.ErrUserNotFound
	}
	return u, nil
}

func (r *memRepo) GetByName(_ context.Context, name string) (identity.User, error) {
	if r.failOn == "GetByName" {
		return identity.User{}, errAuthBoom
	}
	for _, u := range r.byID {
		if u.Name == name {
			return u, nil
		}
	}
	return identity.User{}, outbound.ErrUserNotFound
}

func (r *memRepo) List(_ context.Context) ([]identity.User, error) {
	if r.failOn == "List" {
		return nil, errAuthBoom
	}
	out := make([]identity.User, 0, len(r.byID))
	for _, u := range r.byID {
		out = append(out, u)
	}
	return out, nil
}

func (r *memRepo) Count(_ context.Context) (int, error) {
	if r.failOn == "Count" {
		return 0, errAuthBoom
	}
	return len(r.byID), nil
}

func (r *memRepo) Upsert(_ context.Context, u identity.User) error {
	if r.failOn == "Upsert" {
		return errAuthBoom
	}
	r.byID[u.ID] = u
	return nil
}

func (r *memRepo) Delete(_ context.Context, id string) error {
	if r.failOn == "Delete" {
		return errAuthBoom
	}
	if _, ok := r.byID[id]; !ok {
		return outbound.ErrUserNotFound
	}
	delete(r.byID, id)
	return nil
}

// fakeHasher: hash = "H:"+pw; verify checks that prefix. hashErr forces failure.
type fakeHasher struct{ hashErr error }

func (h fakeHasher) Hash(pw string) (string, error) {
	if h.hashErr != nil {
		return "", h.hashErr
	}
	return "H:" + pw, nil
}
func (h fakeHasher) Verify(pw, hash string) bool { return hash == "H:"+pw }

// fakeIssuer: token = "T:"+id; Parse reverses. issueErr/parseErr force failures.
type fakeIssuer struct {
	issueErr error
	parseErr error
	byID     map[string]identity.Role
}

func newFakeIssuer() *fakeIssuer { return &fakeIssuer{byID: map[string]identity.Role{}} }

func (i *fakeIssuer) Issue(u identity.User) (string, error) {
	if i.issueErr != nil {
		return "", i.issueErr
	}
	i.byID[u.ID] = u.Role
	return "T:" + u.ID, nil
}
func (i *fakeIssuer) Parse(token string) (outbound.Claims, error) {
	if i.parseErr != nil {
		return outbound.Claims{}, i.parseErr
	}
	if len(token) < 2 || token[:2] != "T:" {
		return outbound.Claims{}, errAuthBoom
	}
	id := token[2:]
	return outbound.Claims{UserID: id, Role: i.byID[id]}, nil
}

// helper: fresh service with a seeded admin, returns svc, repo, admin user.
func newAuthWithAdmin(t *testing.T) (*AuthService, *memRepo, *fakeIssuer, identity.User) {
	t.Helper()
	repo := newMemRepo()
	iss := newFakeIssuer()
	svc := NewAuthService(repo, fakeHasher{}, iss)
	admin, err := svc.SeedAdmin(context.Background(), "root", "secret")
	if err != nil {
		t.Fatalf("SeedAdmin: %v", err)
	}
	return svc, repo, iss, admin
}

// --- tests ---

func TestSeedAdminCreatesWhenEmpty(t *testing.T) {
	svc, repo, _, admin := newAuthWithAdmin(t)
	if admin.Role != identity.RoleAdmin || admin.Name != "root" {
		t.Fatalf("seeded %+v", admin)
	}
	if n, _ := repo.Count(context.Background()); n != 1 {
		t.Fatalf("count=%d want 1", n)
	}
	// second seed is a no-op (repo non-empty)
	got, err := svc.SeedAdmin(context.Background(), "other", "x")
	if err != nil || got.ID != "" {
		t.Fatalf("second seed should no-op, got %+v err %v", got, err)
	}
}

func TestSeedAdminCountErr(t *testing.T) {
	repo := newMemRepo()
	repo.failOn = "Count"
	svc := NewAuthService(repo, fakeHasher{}, newFakeIssuer())
	if _, err := svc.SeedAdmin(context.Background(), "root", "s"); !errors.Is(err, errAuthBoom) {
		t.Fatalf("err=%v want boom", err)
	}
}

func TestSeedAdminHashErr(t *testing.T) {
	svc := NewAuthService(newMemRepo(), fakeHasher{hashErr: errAuthBoom}, newFakeIssuer())
	if _, err := svc.SeedAdmin(context.Background(), "root", "s"); !errors.Is(err, errAuthBoom) {
		t.Fatalf("err=%v want boom", err)
	}
}

func TestSeedAdminUpsertErr(t *testing.T) {
	repo := newMemRepo()
	repo.failOn = "Upsert"
	svc := NewAuthService(repo, fakeHasher{}, newFakeIssuer())
	if _, err := svc.SeedAdmin(context.Background(), "root", "s"); !errors.Is(err, errAuthBoom) {
		t.Fatalf("err=%v want boom", err)
	}
}

func TestLoginOK(t *testing.T) {
	svc, _, _, admin := newAuthWithAdmin(t)
	token, u, err := svc.Login(context.Background(), "root", "secret")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if token != "T:"+admin.ID || u.ID != admin.ID {
		t.Fatalf("token=%q u=%+v", token, u)
	}
}

func TestLoginUnknownName(t *testing.T) {
	svc, _, _, _ := newAuthWithAdmin(t)
	if _, _, err := svc.Login(context.Background(), "ghost", "x"); !errors.Is(err, inbound.ErrBadCredentials) {
		t.Fatalf("err=%v want ErrBadCredentials", err)
	}
}

func TestLoginWrongPassword(t *testing.T) {
	svc, _, _, _ := newAuthWithAdmin(t)
	if _, _, err := svc.Login(context.Background(), "root", "nope"); !errors.Is(err, inbound.ErrBadCredentials) {
		t.Fatalf("err=%v want ErrBadCredentials", err)
	}
}

func TestLoginRepoErr(t *testing.T) {
	svc, repo, _, _ := newAuthWithAdmin(t)
	repo.failOn = "GetByName"
	if _, _, err := svc.Login(context.Background(), "root", "secret"); !errors.Is(err, errAuthBoom) {
		t.Fatalf("err=%v want boom", err)
	}
}

func TestLoginIssueErr(t *testing.T) {
	repo := newMemRepo()
	iss := newFakeIssuer()
	svc := NewAuthService(repo, fakeHasher{}, iss)
	svc.SeedAdmin(context.Background(), "root", "secret")
	iss.issueErr = errAuthBoom
	if _, _, err := svc.Login(context.Background(), "root", "secret"); !errors.Is(err, errAuthBoom) {
		t.Fatalf("err=%v want boom", err)
	}
}

func TestMeOK(t *testing.T) {
	svc, _, _, admin := newAuthWithAdmin(t)
	u, err := svc.Me(context.Background(), "T:"+admin.ID)
	if err != nil || u.ID != admin.ID {
		t.Fatalf("Me u=%+v err=%v", u, err)
	}
}

func TestMeBadToken(t *testing.T) {
	svc, _, _, _ := newAuthWithAdmin(t)
	if _, err := svc.Me(context.Background(), "garbage"); !errors.Is(err, inbound.ErrBadToken) {
		t.Fatalf("err=%v want ErrBadToken", err)
	}
}

func TestMeUserGone(t *testing.T) {
	svc, repo, _, admin := newAuthWithAdmin(t)
	delete(repo.byID, admin.ID)
	if _, err := svc.Me(context.Background(), "T:"+admin.ID); !errors.Is(err, inbound.ErrUserNotFound) {
		t.Fatalf("err=%v want ErrUserNotFound", err)
	}
}

func TestMeRepoErr(t *testing.T) {
	svc, repo, _, admin := newAuthWithAdmin(t)
	repo.failOn = "Get"
	if _, err := svc.Me(context.Background(), "T:"+admin.ID); !errors.Is(err, errAuthBoom) {
		t.Fatalf("err=%v want boom", err)
	}
}

func TestAuthorizeOK(t *testing.T) {
	svc, _, _, admin := newAuthWithAdmin(t)
	u, err := svc.Authorize(context.Background(), "T:"+admin.ID, identity.PermUsersManage)
	if err != nil || u.ID != admin.ID {
		t.Fatalf("Authorize u=%+v err=%v", u, err)
	}
}

func TestAuthorizeForbidden(t *testing.T) {
	svc, _, _, admin := newAuthWithAdmin(t)
	// create an announcer, then authorize a users.manage perm it lacks
	an, err := svc.CreateUser(context.Background(), admin, "Léo", identity.RoleAnnouncer, "pw")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if _, err := svc.Authorize(context.Background(), "T:"+an.ID, identity.PermUsersManage); !errors.Is(err, inbound.ErrForbidden) {
		t.Fatalf("err=%v want ErrForbidden", err)
	}
	// but the announcer CAN control playback
	if _, err := svc.Authorize(context.Background(), "T:"+an.ID, identity.PermPlaybackControl); err != nil {
		t.Fatalf("announcer playback: %v", err)
	}
}

func TestAuthorizeBadToken(t *testing.T) {
	svc, _, _, _ := newAuthWithAdmin(t)
	if _, err := svc.Authorize(context.Background(), "x", identity.PermReportsRead); !errors.Is(err, inbound.ErrBadToken) {
		t.Fatalf("err=%v want ErrBadToken", err)
	}
}

func TestListUsersOK(t *testing.T) {
	svc, _, _, admin := newAuthWithAdmin(t)
	got, err := svc.ListUsers(context.Background(), admin)
	if err != nil || len(got) != 1 {
		t.Fatalf("ListUsers got %d err %v", len(got), err)
	}
}

func TestListUsersForbidden(t *testing.T) {
	svc, _, _, admin := newAuthWithAdmin(t)
	an, _ := svc.CreateUser(context.Background(), admin, "Léo", identity.RoleAnnouncer, "pw")
	if _, err := svc.ListUsers(context.Background(), an); !errors.Is(err, inbound.ErrForbidden) {
		t.Fatalf("err=%v want ErrForbidden", err)
	}
}

func TestListUsersRepoErr(t *testing.T) {
	svc, repo, _, admin := newAuthWithAdmin(t)
	repo.failOn = "List"
	if _, err := svc.ListUsers(context.Background(), admin); !errors.Is(err, errAuthBoom) {
		t.Fatalf("err=%v want boom", err)
	}
}

func TestCreateUserOK(t *testing.T) {
	svc, _, _, admin := newAuthWithAdmin(t)
	u, err := svc.CreateUser(context.Background(), admin, "Ana", identity.RoleOperator, "pw")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if u.Role != identity.RoleOperator || u.PassHash != "H:pw" {
		t.Fatalf("created %+v", u)
	}
}

func TestCreateUserForbidden(t *testing.T) {
	svc, _, _, admin := newAuthWithAdmin(t)
	an, _ := svc.CreateUser(context.Background(), admin, "Léo", identity.RoleAnnouncer, "pw")
	if _, err := svc.CreateUser(context.Background(), an, "X", identity.RoleOperator, "pw"); !errors.Is(err, inbound.ErrForbidden) {
		t.Fatalf("err=%v want ErrForbidden", err)
	}
}

func TestCreateUserBadRole(t *testing.T) {
	svc, _, _, admin := newAuthWithAdmin(t)
	if _, err := svc.CreateUser(context.Background(), admin, "X", identity.Role("root"), "pw"); !errors.Is(err, identity.ErrBadRole) {
		t.Fatalf("err=%v want ErrBadRole", err)
	}
}

func TestCreateUserEmptyName(t *testing.T) {
	svc, _, _, admin := newAuthWithAdmin(t)
	if _, err := svc.CreateUser(context.Background(), admin, "  ", identity.RoleOperator, "pw"); !errors.Is(err, identity.ErrEmptyName) {
		t.Fatalf("err=%v want ErrEmptyName", err)
	}
}

func TestCreateUserNameTaken(t *testing.T) {
	svc, _, _, admin := newAuthWithAdmin(t)
	if _, err := svc.CreateUser(context.Background(), admin, "root", identity.RoleOperator, "pw"); !errors.Is(err, inbound.ErrNameTaken) {
		t.Fatalf("err=%v want ErrNameTaken", err)
	}
}

func TestCreateUserLookupErr(t *testing.T) {
	svc, repo, _, admin := newAuthWithAdmin(t)
	repo.failOn = "GetByName"
	if _, err := svc.CreateUser(context.Background(), admin, "Ana", identity.RoleOperator, "pw"); !errors.Is(err, errAuthBoom) {
		t.Fatalf("err=%v want boom", err)
	}
}

func TestCreateUserHashErr(t *testing.T) {
	repo := newMemRepo()
	svc := NewAuthService(repo, fakeHasher{}, newFakeIssuer())
	admin, _ := svc.SeedAdmin(context.Background(), "root", "s")
	// swap hasher to a failing one
	svc.hasher = fakeHasher{hashErr: errAuthBoom}
	if _, err := svc.CreateUser(context.Background(), admin, "Ana", identity.RoleOperator, "pw"); !errors.Is(err, errAuthBoom) {
		t.Fatalf("err=%v want boom", err)
	}
}

func TestCreateUserUpsertErr(t *testing.T) {
	svc, repo, _, admin := newAuthWithAdmin(t)
	repo.failOn = "Upsert"
	if _, err := svc.CreateUser(context.Background(), admin, "Ana", identity.RoleOperator, "pw"); !errors.Is(err, errAuthBoom) {
		t.Fatalf("err=%v want boom", err)
	}
}

func TestUpdateUserRoleOnly(t *testing.T) {
	svc, _, _, admin := newAuthWithAdmin(t)
	u, _ := svc.CreateUser(context.Background(), admin, "Ana", identity.RoleOperator, "pw")
	up, err := svc.UpdateUser(context.Background(), admin, u.ID, identity.RoleAnnouncer, "")
	if err != nil {
		t.Fatalf("UpdateUser: %v", err)
	}
	if up.Role != identity.RoleAnnouncer || up.PassHash != "H:pw" {
		t.Fatalf("updated %+v (hash should be unchanged)", up)
	}
}

func TestUpdateUserWithPassword(t *testing.T) {
	svc, _, _, admin := newAuthWithAdmin(t)
	u, _ := svc.CreateUser(context.Background(), admin, "Ana", identity.RoleOperator, "pw")
	up, err := svc.UpdateUser(context.Background(), admin, u.ID, identity.RoleOperator, "new")
	if err != nil || up.PassHash != "H:new" {
		t.Fatalf("updated %+v err %v", up, err)
	}
}

func TestUpdateUserForbidden(t *testing.T) {
	svc, _, _, admin := newAuthWithAdmin(t)
	an, _ := svc.CreateUser(context.Background(), admin, "Léo", identity.RoleAnnouncer, "pw")
	if _, err := svc.UpdateUser(context.Background(), an, admin.ID, identity.RoleOperator, ""); !errors.Is(err, inbound.ErrForbidden) {
		t.Fatalf("err=%v want ErrForbidden", err)
	}
}

func TestUpdateUserNotFound(t *testing.T) {
	svc, _, _, admin := newAuthWithAdmin(t)
	if _, err := svc.UpdateUser(context.Background(), admin, "ghost", identity.RoleOperator, ""); !errors.Is(err, inbound.ErrUserNotFound) {
		t.Fatalf("err=%v want ErrUserNotFound", err)
	}
}

func TestUpdateUserGetErr(t *testing.T) {
	svc, repo, _, admin := newAuthWithAdmin(t)
	u, _ := svc.CreateUser(context.Background(), admin, "Ana", identity.RoleOperator, "pw")
	repo.failOn = "Get"
	if _, err := svc.UpdateUser(context.Background(), admin, u.ID, identity.RoleOperator, ""); !errors.Is(err, errAuthBoom) {
		t.Fatalf("err=%v want boom", err)
	}
}

func TestUpdateUserHashErr(t *testing.T) {
	svc, _, _, admin := newAuthWithAdmin(t)
	u, _ := svc.CreateUser(context.Background(), admin, "Ana", identity.RoleOperator, "pw")
	svc.hasher = fakeHasher{hashErr: errAuthBoom}
	if _, err := svc.UpdateUser(context.Background(), admin, u.ID, identity.RoleOperator, "new"); !errors.Is(err, errAuthBoom) {
		t.Fatalf("err=%v want boom", err)
	}
}

func TestUpdateUserBadRole(t *testing.T) {
	svc, _, _, admin := newAuthWithAdmin(t)
	u, _ := svc.CreateUser(context.Background(), admin, "Ana", identity.RoleOperator, "pw")
	if _, err := svc.UpdateUser(context.Background(), admin, u.ID, identity.Role("root"), ""); !errors.Is(err, identity.ErrBadRole) {
		t.Fatalf("err=%v want ErrBadRole", err)
	}
}

func TestUpdateUserUpsertErr(t *testing.T) {
	svc, repo, _, admin := newAuthWithAdmin(t)
	u, _ := svc.CreateUser(context.Background(), admin, "Ana", identity.RoleOperator, "pw")
	repo.failOn = "Upsert"
	if _, err := svc.UpdateUser(context.Background(), admin, u.ID, identity.RoleAnnouncer, ""); !errors.Is(err, errAuthBoom) {
		t.Fatalf("err=%v want boom", err)
	}
}

func TestDeleteUserOK(t *testing.T) {
	svc, repo, _, admin := newAuthWithAdmin(t)
	u, _ := svc.CreateUser(context.Background(), admin, "Ana", identity.RoleOperator, "pw")
	if err := svc.DeleteUser(context.Background(), admin, u.ID); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
	if _, ok := repo.byID[u.ID]; ok {
		t.Fatal("user not deleted")
	}
}

func TestDeleteUserForbidden(t *testing.T) {
	svc, _, _, admin := newAuthWithAdmin(t)
	an, _ := svc.CreateUser(context.Background(), admin, "Léo", identity.RoleAnnouncer, "pw")
	if err := svc.DeleteUser(context.Background(), an, admin.ID); !errors.Is(err, inbound.ErrForbidden) {
		t.Fatalf("err=%v want ErrForbidden", err)
	}
}

func TestDeleteUserNotFound(t *testing.T) {
	svc, _, _, admin := newAuthWithAdmin(t)
	if err := svc.DeleteUser(context.Background(), admin, "ghost"); !errors.Is(err, inbound.ErrUserNotFound) {
		t.Fatalf("err=%v want ErrUserNotFound", err)
	}
}

func TestDeleteUserRepoErr(t *testing.T) {
	svc, repo, _, admin := newAuthWithAdmin(t)
	u, _ := svc.CreateUser(context.Background(), admin, "Ana", identity.RoleOperator, "pw")
	repo.failOn = "Delete"
	if err := svc.DeleteUser(context.Background(), admin, u.ID); !errors.Is(err, errAuthBoom) {
		t.Fatalf("err=%v want boom", err)
	}
}
