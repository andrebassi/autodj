package sqlite

import (
	"context"
	"testing"
	"time"

	"autodj/domain/youtube"
)

func ytAcctFixture() youtube.Account {
	return youtube.Account{
		AccessToken:  "acc-tok",
		RefreshToken: "ref-tok",
		ExpiresAt:    time.Unix(1_800_000_000, 0).UTC(),
		Scopes:       []string{"https://www.googleapis.com/auth/youtube.readonly", "openid"},
		ChannelTitle: "My Channel",
		Premium:      true,
	}
}

func TestYouTubeAccountRepoSQLite(t *testing.T) {
	repo, closeFn, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer closeFn()
	yr := repo.YouTubeAccount()
	ctx := context.Background()

	// unlinked when never saved.
	if _, err := yr.Load(ctx); err != youtube.ErrNoAccount {
		t.Fatalf("empty Load = %v, want ErrNoAccount", err)
	}

	// save then read back.
	if err := yr.Save(ctx, ytAcctFixture()); err != nil {
		t.Fatalf("Save: %v", err)
	}
	a, err := yr.Load(ctx)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if a.AccessToken != "acc-tok" || a.ChannelTitle != "My Channel" || !a.IsPremium() ||
		!a.HasScope("openid") {
		t.Fatalf("account = %+v", a)
	}

	// save again replaces (single-row upsert).
	if err := yr.Save(ctx, youtube.Account{AccessToken: "new"}); err != nil {
		t.Fatalf("Save replace: %v", err)
	}
	a2, _ := yr.Load(ctx)
	if a2.AccessToken != "new" || a2.IsPremium() {
		t.Fatalf("replace = %+v", a2)
	}

	// delete unlinks.
	if err := yr.Delete(ctx); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := yr.Load(ctx); err != youtube.ErrNoAccount {
		t.Fatalf("after Delete = %v, want ErrNoAccount", err)
	}
	// delete idempotent.
	if err := yr.Delete(ctx); err != nil {
		t.Fatalf("second Delete: %v", err)
	}
}

func TestYouTubeAccountRepoSQLiteDecodeError(t *testing.T) {
	repo, closeFn, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer closeFn()
	if _, err := repo.db.Exec(`INSERT INTO youtube_account (id, data) VALUES (1, ?)`, "{not json"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := repo.YouTubeAccount().Load(context.Background()); err == nil {
		t.Fatal("expected decode error on Load")
	}
}

func TestYouTubeAccountRepoSQLiteClosedDB(t *testing.T) {
	repo, closeFn, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	yr := repo.YouTubeAccount()
	closeFn()
	ctx := context.Background()
	if _, err := yr.Load(ctx); err == nil {
		t.Fatal("expected Load error on closed db")
	}
	if err := yr.Save(ctx, ytAcctFixture()); err == nil {
		t.Fatal("expected Save error on closed db")
	}
	if err := yr.Delete(ctx); err == nil {
		t.Fatal("expected Delete error on closed db")
	}
}
