package sqlite

import (
	"context"
	"testing"
	"time"

	"autodj/domain/spotify"
)

func acctFixture() spotify.Account {
	return spotify.Account{
		AccessToken:  "acc-tok",
		RefreshToken: "ref-tok",
		ExpiresAt:    time.Unix(1_800_000_000, 0).UTC(),
		Scopes:       []string{"streaming", "user-read-private"},
		Product:      "premium",
	}
}

func TestSpotifyAccountRepoSQLite(t *testing.T) {
	repo, closeFn, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer closeFn()
	sr := repo.SpotifyAccount()
	ctx := context.Background()

	// unlinked when never saved.
	if _, err := sr.Load(ctx); err != spotify.ErrNoAccount {
		t.Fatalf("empty Load = %v, want ErrNoAccount", err)
	}

	// save then read back.
	if err := sr.Save(ctx, acctFixture()); err != nil {
		t.Fatalf("Save: %v", err)
	}
	a, err := sr.Load(ctx)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if a.AccessToken != "acc-tok" || !a.Premium() || !a.HasScope("streaming") {
		t.Fatalf("account = %+v", a)
	}

	// save again replaces (single-row upsert).
	if err := sr.Save(ctx, spotify.Account{AccessToken: "new", Product: "free"}); err != nil {
		t.Fatalf("Save replace: %v", err)
	}
	a2, _ := sr.Load(ctx)
	if a2.AccessToken != "new" || a2.Premium() {
		t.Fatalf("replace = %+v", a2)
	}

	// delete unlinks.
	if err := sr.Delete(ctx); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := sr.Load(ctx); err != spotify.ErrNoAccount {
		t.Fatalf("after Delete = %v, want ErrNoAccount", err)
	}
	// delete idempotent.
	if err := sr.Delete(ctx); err != nil {
		t.Fatalf("second Delete: %v", err)
	}
}

func TestSpotifyAccountRepoSQLiteDecodeError(t *testing.T) {
	repo, closeFn, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer closeFn()
	if _, err := repo.db.Exec(`INSERT INTO spotify_account (id, data) VALUES (1, ?)`, "{not json"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := repo.SpotifyAccount().Load(context.Background()); err == nil {
		t.Fatal("expected decode error on Load")
	}
}

func TestSpotifyAccountRepoSQLiteClosedDB(t *testing.T) {
	repo, closeFn, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	sr := repo.SpotifyAccount()
	closeFn()
	ctx := context.Background()
	if _, err := sr.Load(ctx); err == nil {
		t.Fatal("expected Load error on closed db")
	}
	if err := sr.Save(ctx, acctFixture()); err == nil {
		t.Fatal("expected Save error on closed db")
	}
	if err := sr.Delete(ctx); err == nil {
		t.Fatal("expected Delete error on closed db")
	}
}
