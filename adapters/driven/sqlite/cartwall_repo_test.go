package sqlite

import (
	"context"
	"testing"

	"autodj/domain/cartwall"
)

func cartFixture() cartwall.Board {
	return cartwall.Board{Buttons: []cartwall.CartButton{
		{ID: "a", Label: "Vinheta", TrackID: "demo-01", Hotkey: "q", Page: 1},
		{ID: "b", Label: "Efeito", TrackID: "demo-02", Hotkey: "w", Page: 1},
	}}
}

func TestCartwallRepoSQLite(t *testing.T) {
	repo, closeFn, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer closeFn()
	cr := repo.Cartwall()
	ctx := context.Background()

	// empty when never saved.
	if b, err := cr.Load(ctx); err != nil || len(b.Buttons) != 0 {
		t.Fatalf("Load empty = %+v, %v", b, err)
	}

	// save then read back.
	if err := cr.Save(ctx, cartFixture()); err != nil {
		t.Fatalf("Save: %v", err)
	}
	b, err := cr.Load(ctx)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(b.Buttons) != 2 {
		t.Fatalf("buttons = %d, want 2", len(b.Buttons))
	}

	// save again replaces (single-row upsert).
	if err := cr.Save(ctx, cartwall.Board{Buttons: []cartwall.CartButton{
		{ID: "z", Label: "Solo", TrackID: "demo-03", Hotkey: "p", Page: 2},
	}}); err != nil {
		t.Fatalf("Save replace: %v", err)
	}
	b2, _ := cr.Load(ctx)
	if len(b2.Buttons) != 1 || b2.Buttons[0].ID != "z" {
		t.Fatalf("replace board = %+v", b2)
	}
}

func TestCartwallRepoSQLiteDecodeError(t *testing.T) {
	repo, closeFn, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer closeFn()
	if _, err := repo.db.Exec(`INSERT INTO cartwall (id, data) VALUES (1, ?)`, "{not json"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := repo.Cartwall().Load(context.Background()); err == nil {
		t.Fatal("expected decode error on Load")
	}
}

func TestCartwallRepoSQLiteClosedDB(t *testing.T) {
	repo, closeFn, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	cr := repo.Cartwall()
	closeFn()
	ctx := context.Background()
	if _, err := cr.Load(ctx); err == nil {
		t.Fatal("expected Load error on closed db")
	}
	if err := cr.Save(ctx, cartFixture()); err == nil {
		t.Fatal("expected Save error on closed db")
	}
}
