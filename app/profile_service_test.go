package app

import (
	"context"
	"errors"
	"testing"

	"autodj/domain/identity"
	"autodj/domain/operatorprofile"
	"autodj/ports/inbound"
	"autodj/ports/outbound"
)

// --- fake ProfileRepo ---

type memProfileRepo struct {
	byUser map[string]operatorprofile.Profile
	failOn string // "Get" | "Upsert" → errProfBoom
}

var errProfBoom = errors.New("boom")

func newMemProfileRepo() *memProfileRepo {
	return &memProfileRepo{byUser: map[string]operatorprofile.Profile{}}
}

func (r *memProfileRepo) Get(_ context.Context, userID string) (operatorprofile.Profile, error) {
	if r.failOn == "Get" {
		return operatorprofile.Profile{}, errProfBoom
	}
	p, ok := r.byUser[userID]
	if !ok {
		return operatorprofile.Profile{}, outbound.ErrProfileNotFound
	}
	return p, nil
}

func (r *memProfileRepo) Upsert(_ context.Context, p operatorprofile.Profile) error {
	if r.failOn == "Upsert" {
		return errProfBoom
	}
	r.byUser[p.UserID] = p
	return nil
}

func mkUser(id string, role identity.Role) identity.User {
	u, _ := identity.NewUser(id, "u-"+id, role, "h")
	return u
}

// --- tests ---

func TestProfileGetDefaultWhenAbsent(t *testing.T) {
	svc := NewProfileService(newMemProfileRepo())
	got, err := svc.Get(context.Background(), "usr_x")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	want := operatorprofile.Default("usr_x")
	if got.UserID != want.UserID || got.Theme != want.Theme || len(got.Panels) != len(want.Panels) {
		t.Fatalf("get default mismatch: %+v", got)
	}
}

func TestProfileGetStored(t *testing.T) {
	repo := newMemProfileRepo()
	p := operatorprofile.Default("usr_1")
	p.Theme = operatorprofile.ThemeLight
	repo.byUser["usr_1"] = p
	svc := NewProfileService(repo)
	got, err := svc.Get(context.Background(), "usr_1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Theme != operatorprofile.ThemeLight {
		t.Fatalf("theme = %v, want claro", got.Theme)
	}
}

func TestProfileGetRepoError(t *testing.T) {
	repo := newMemProfileRepo()
	repo.failOn = "Get"
	svc := NewProfileService(repo)
	if _, err := svc.Get(context.Background(), "u"); !errors.Is(err, errProfBoom) {
		t.Fatalf("err = %v, want boom", err)
	}
}

func TestProfileSaveOwner(t *testing.T) {
	repo := newMemProfileRepo()
	svc := NewProfileService(repo)
	actor := mkUser("usr_1", identity.RoleOperator)
	p := operatorprofile.Default("usr_1")
	p.Toggle(operatorprofile.PanelLibrary)
	got, err := svc.Save(context.Background(), actor, p)
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if _, ok := repo.byUser["usr_1"]; !ok {
		t.Fatalf("profile not persisted")
	}
	// order stays normalized
	for i, pref := range got.Panels {
		if pref.Order != i {
			t.Errorf("panel %d order = %d", i, pref.Order)
		}
	}
}

func TestProfileSaveAdminForOther(t *testing.T) {
	svc := NewProfileService(newMemProfileRepo())
	admin := mkUser("usr_admin", identity.RoleAdmin) // holds users.manage
	p := operatorprofile.Default("usr_other")
	if _, err := svc.Save(context.Background(), admin, p); err != nil {
		t.Fatalf("admin save for other: %v", err)
	}
}

func TestProfileSaveForbiddenForOther(t *testing.T) {
	svc := NewProfileService(newMemProfileRepo())
	op := mkUser("usr_1", identity.RoleOperator) // no users.manage
	p := operatorprofile.Default("usr_2")
	if _, err := svc.Save(context.Background(), op, p); !errors.Is(err, inbound.ErrForbidden) {
		t.Fatalf("err = %v, want ErrForbidden", err)
	}
}

func TestProfileSaveValidationError(t *testing.T) {
	svc := NewProfileService(newMemProfileRepo())
	actor := mkUser("usr_1", identity.RoleOperator)
	p := operatorprofile.Default("usr_1")
	p.Theme = "neon"
	if _, err := svc.Save(context.Background(), actor, p); !errors.Is(err, operatorprofile.ErrBadTheme) {
		t.Fatalf("err = %v, want ErrBadTheme", err)
	}
}

func TestProfileSaveRepoError(t *testing.T) {
	repo := newMemProfileRepo()
	repo.failOn = "Upsert"
	svc := NewProfileService(repo)
	actor := mkUser("usr_1", identity.RoleOperator)
	p := operatorprofile.Default("usr_1")
	if _, err := svc.Save(context.Background(), actor, p); !errors.Is(err, errProfBoom) {
		t.Fatalf("err = %v, want boom", err)
	}
}
