package sqlite

import (
	"context"
	"testing"

	"autodj/domain/mixing"
)

func TestMixRepoSQLite(t *testing.T) {
	repo, closeFn, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer closeFn()
	mix := repo.Mixing()
	ctx := context.Background()

	// No row yet → factory defaults.
	got, err := mix.Get(ctx)
	if err != nil {
		t.Fatalf("Get(empty): %v", err)
	}
	if got != mixing.Default() {
		t.Fatalf("empty Get = %+v, want default", got)
	}

	// Save then read back.
	want := mixing.Default()
	want.MusicaOut = 4200
	want.Normalize = false
	want.TalkVol = 12
	if err := mix.Save(ctx, want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err = mix.Get(ctx)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != want {
		t.Fatalf("Get = %+v, want %+v", got, want)
	}

	// Second save = update (still one row).
	want.OverlapVol = 25
	if err := mix.Save(ctx, want); err != nil {
		t.Fatalf("Save#2: %v", err)
	}
	got, _ = mix.Get(ctx)
	if got.OverlapVol != 25 {
		t.Fatalf("OverlapVol = %d, want 25", got.OverlapVol)
	}
}

func TestMixRepoSQLiteDecodeError(t *testing.T) {
	repo, closeFn, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer closeFn()
	// Corrupt the stored blob → Get must surface a decode error.
	if _, err := repo.db.Exec(`INSERT INTO mix_settings (id, data) VALUES (1, ?)`, "{not json"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := repo.Mixing().Get(context.Background()); err == nil {
		t.Fatal("expected decode error")
	}
}

func TestMixRepoSQLiteClosedDB(t *testing.T) {
	repo, closeFn, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	mix := repo.Mixing()
	closeFn() // now every query fails
	ctx := context.Background()
	if _, err := mix.Get(ctx); err == nil {
		t.Fatal("expected Get error on closed db")
	}
	if err := mix.Save(ctx, mixing.Default()); err == nil {
		t.Fatal("expected Save error on closed db")
	}
}
