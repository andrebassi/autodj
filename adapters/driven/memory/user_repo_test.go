package memory

import (
	"context"
	"errors"
	"testing"

	"autodj/domain/identity"
	"autodj/ports/outbound"
)

func sampleUser(id, name string, role identity.Role) identity.User {
	return identity.User{ID: id, Name: name, Role: role, PassHash: "h:" + id}
}

func TestUserRepoMemory(t *testing.T) {
	ctx := context.Background()
	r := NewUserRepo()

	// empty
	if n, _ := r.Count(ctx); n != 0 {
		t.Fatalf("Count empty = %d", n)
	}
	if list, err := r.List(ctx); err != nil || len(list) != 0 {
		t.Fatalf("List empty = %v, %v", list, err)
	}
	if _, err := r.Get(ctx, "nope"); !errors.Is(err, outbound.ErrUserNotFound) {
		t.Fatalf("Get missing = %v", err)
	}
	if _, err := r.GetByName(ctx, "nope"); !errors.Is(err, outbound.ErrUserNotFound) {
		t.Fatalf("GetByName missing = %v", err)
	}
	if err := r.Delete(ctx, "nope"); !errors.Is(err, outbound.ErrUserNotFound) {
		t.Fatalf("Delete missing = %v", err)
	}

	// upsert two, list ordered by id
	if err := r.Upsert(ctx, sampleUser("b", "Bruno", identity.RoleOperator)); err != nil {
		t.Fatal(err)
	}
	if err := r.Upsert(ctx, sampleUser("a", "Ana", identity.RoleAdmin)); err != nil {
		t.Fatal(err)
	}
	list, _ := r.List(ctx)
	if len(list) != 2 || list[0].ID != "a" || list[1].ID != "b" {
		t.Fatalf("List = %v", list)
	}
	if n, _ := r.Count(ctx); n != 2 {
		t.Fatalf("Count = %d want 2", n)
	}

	// get + getByName
	if u, err := r.Get(ctx, "a"); err != nil || u.Name != "Ana" {
		t.Fatalf("Get a = %v, %v", u, err)
	}
	if u, err := r.GetByName(ctx, "Bruno"); err != nil || u.ID != "b" {
		t.Fatalf("GetByName Bruno = %v, %v", u, err)
	}

	// upsert (update role)
	if err := r.Upsert(ctx, sampleUser("a", "Ana", identity.RoleAnnouncer)); err != nil {
		t.Fatal(err)
	}
	if u, _ := r.Get(ctx, "a"); u.Role != identity.RoleAnnouncer {
		t.Fatalf("role after upsert = %q", u.Role)
	}

	// delete
	if err := r.Delete(ctx, "a"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Get(ctx, "a"); !errors.Is(err, outbound.ErrUserNotFound) {
		t.Fatalf("Get after delete = %v", err)
	}
}
