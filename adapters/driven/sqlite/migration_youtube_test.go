package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"autodj/domain/library"
	"autodj/domain/shared"
)

// A DB created before YouTube carries the old source CHECK (rejecting 'youtube')
// and lacks the youtube_uri column. Open must add the column AND rebuild the
// tracks/play_log tables so a youtube row inserts — the ensureSourceCheck path.
func TestEnsureSourceCheckMigratesLegacyDB(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "legacy.db")

	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	// Legacy schema: source CHECK WITHOUT 'youtube', no youtube_uri/markers/artwork_url
	// (those arrive via ALTER before the rebuild). created_at present so the rebuild
	// INSERT...SELECT copies every column.
	const legacyTracks = `CREATE TABLE tracks (
	  id TEXT PRIMARY KEY, title TEXT NOT NULL DEFAULT '', artist TEXT NOT NULL DEFAULT '',
	  album TEXT NOT NULL DEFAULT '', category TEXT NOT NULL DEFAULT 'music', source TEXT NOT NULL,
	  path TEXT NOT NULL DEFAULT '', spotify_uri TEXT NOT NULL DEFAULT '',
	  duration_ms INTEGER NOT NULL DEFAULT 0, bpm REAL NOT NULL DEFAULT 0,
	  musical_key TEXT NOT NULL DEFAULT '', waveform_ref TEXT NOT NULL DEFAULT '',
	  play_count INTEGER NOT NULL DEFAULT 0, pcm_available INTEGER NOT NULL DEFAULT 0,
	  mixable INTEGER NOT NULL DEFAULT 0, created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	  CONSTRAINT source_valid CHECK (source IN ('local','spotify','line_in','tts')),
	  CONSTRAINT spotify_never_mixable CHECK (NOT (source = 'spotify' AND mixable = 1)));`
	const legacyLog = `CREATE TABLE play_log (
	  id TEXT PRIMARY KEY, track_id TEXT NOT NULL, title TEXT NOT NULL DEFAULT '',
	  artist TEXT NOT NULL DEFAULT '', source TEXT NOT NULL, played_at TEXT NOT NULL,
	  duration_ms INTEGER NOT NULL DEFAULT 0, broadcasted INTEGER NOT NULL DEFAULT 0,
	  ecad_relevant INTEGER NOT NULL DEFAULT 0,
	  CONSTRAINT play_log_source_valid CHECK (source IN ('local','spotify','line_in','tts')));`
	for _, q := range []string{legacyTracks, legacyLog} {
		if _, err := raw.ExecContext(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	// A pre-existing local row must survive the rebuild.
	if _, err := raw.ExecContext(ctx,
		`INSERT INTO tracks (id, source, path, duration_ms, pcm_available, mixable)
		 VALUES ('old','local','/o',1000,1,1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.ExecContext(ctx,
		`INSERT INTO play_log (id, track_id, title, source, played_at)
		 VALUES ('l1','old','Old','local','2026-07-24T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	_ = raw.Close()

	// Open migrates: ensureYouTubeColumn + ensureSourceCheck rebuild.
	r, closeFn, err := Open(path)
	if err != nil {
		t.Fatalf("open must migrate legacy DB: %v", err)
	}
	defer closeFn()

	// The legacy row survived.
	if got, err := r.Get(ctx, "old"); err != nil || got.ID != "old" {
		t.Fatalf("legacy track lost in rebuild: got=%+v err=%v", got, err)
	}
	// play_log row survived the rebuild.
	var n int
	if err := r.db.QueryRowContext(ctx, `SELECT count(*) FROM play_log`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("play_log row lost: n=%d err=%v", n, err)
	}

	// The relaxed CHECK now accepts a youtube track (would have failed pre-rebuild).
	yt, err := library.NewTrack("y", shared.SourceYouTube, "Vid", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	yt.YouTubeURI = "https://youtu.be/abc"
	if err := r.Upsert(ctx, yt); err != nil {
		t.Fatalf("youtube upsert must pass relaxed CHECK: %v", err)
	}
	got, err := r.Get(ctx, "y")
	if err != nil || got.Source != shared.SourceYouTube || got.YouTubeURI != "https://youtu.be/abc" {
		t.Fatalf("youtube round-trip: got=%+v err=%v", got, err)
	}
	if got.Mixable || got.PCMAvailable {
		t.Fatalf("youtube must be non-mixable/non-PCM: %+v", got)
	}

	// Re-open is idempotent: the CHECK already has 'youtube' → no second rebuild.
	if err := ensureSourceCheck(r.db); err != nil {
		t.Fatalf("second ensureSourceCheck must be a no-op: %v", err)
	}
}

// The DB CHECK is defense-in-depth: even bypassing the domain, a mixable youtube
// row must be rejected by youtube_never_mixable after the migration.
func TestYouTubeNeverMixableCheck(t *testing.T) {
	ctx := context.Background()
	r := openTmp(t)
	const q = `INSERT INTO tracks (id, source, youtube_uri, duration_ms, mixable)
	           VALUES (?, 'youtube', 'https://youtu.be/z', 1000, 1)`
	if _, err := r.db.ExecContext(ctx, q, "z"); err == nil {
		t.Fatal("CHECK youtube_never_mixable must reject a mixable youtube row")
	}
}

// ensureSourceCheck must surface DB errors from tableDefLacks (closed DB fails the
// sqlite_master query) — covers the error-return branch.
func TestEnsureSourceCheckClosedDB(t *testing.T) {
	closed, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	_ = closed.Close()
	if err := ensureSourceCheck(closed); err == nil {
		t.Fatal("ensureSourceCheck on closed db must error")
	}
}

// When the rebuild fires but the legacy table is missing a column the rebuild
// copies, the transactional INSERT...SELECT fails and the error surfaces (the tx
// rolls back). Covers the rebuild-error branch inside ensureSourceCheck.
func TestEnsureSourceCheckRebuildError(t *testing.T) {
	ctx := context.Background()
	raw, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	// Old CHECK (no 'youtube') so needTracks=true, but missing bpm/waveform_ref/etc.
	// that rebuildTracksSQL SELECTs → INSERT...SELECT fails mid-rebuild.
	if _, err := raw.ExecContext(ctx, `CREATE TABLE tracks (
	  id TEXT PRIMARY KEY, source TEXT NOT NULL,
	  CONSTRAINT source_valid CHECK (source IN ('local','spotify')));`); err != nil {
		t.Fatal(err)
	}
	if err := ensureSourceCheck(raw); err == nil {
		t.Fatal("rebuild over an incompatible legacy table must error")
	}
	// The original table must still be intact (transaction rolled back).
	if _, err := raw.ExecContext(ctx, `SELECT id FROM tracks LIMIT 0`); err != nil {
		t.Fatalf("tracks must survive the rolled-back rebuild: %v", err)
	}
}

// Only play_log carries the stale CHECK (tracks already relaxed) → the needLog-only
// rebuild branch runs. Exercises ensureSourceCheck with needTracks=false.
func TestEnsureSourceCheckPlayLogOnly(t *testing.T) {
	ctx := context.Background()
	raw, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	// tracks already has 'youtube' in its CHECK → needTracks=false.
	if _, err := raw.ExecContext(ctx, `CREATE TABLE tracks (
	  id TEXT PRIMARY KEY, track_id TEXT, title TEXT NOT NULL DEFAULT '',
	  artist TEXT NOT NULL DEFAULT '', album TEXT NOT NULL DEFAULT '',
	  category TEXT NOT NULL DEFAULT 'music', source TEXT NOT NULL,
	  path TEXT NOT NULL DEFAULT '', spotify_uri TEXT NOT NULL DEFAULT '',
	  youtube_uri TEXT NOT NULL DEFAULT '', duration_ms INTEGER NOT NULL DEFAULT 0,
	  bpm REAL NOT NULL DEFAULT 0, musical_key TEXT NOT NULL DEFAULT '',
	  waveform_ref TEXT NOT NULL DEFAULT '', play_count INTEGER NOT NULL DEFAULT 0,
	  pcm_available INTEGER NOT NULL DEFAULT 0, mixable INTEGER NOT NULL DEFAULT 0,
	  markers TEXT NOT NULL DEFAULT '[]', artwork_url TEXT NOT NULL DEFAULT '',
	  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	  CONSTRAINT source_valid CHECK (source IN ('local','spotify','line_in','tts','youtube')));`); err != nil {
		t.Fatal(err)
	}
	// play_log still carries the OLD CHECK (no 'youtube') → needLog=true.
	if _, err := raw.ExecContext(ctx, `CREATE TABLE play_log (
	  id TEXT PRIMARY KEY, track_id TEXT NOT NULL, title TEXT NOT NULL DEFAULT '',
	  artist TEXT NOT NULL DEFAULT '', source TEXT NOT NULL, played_at TEXT NOT NULL,
	  duration_ms INTEGER NOT NULL DEFAULT 0, broadcasted INTEGER NOT NULL DEFAULT 0,
	  ecad_relevant INTEGER NOT NULL DEFAULT 0,
	  CONSTRAINT play_log_source_valid CHECK (source IN ('local','spotify','line_in','tts')));`); err != nil {
		t.Fatal(err)
	}
	if err := ensureSourceCheck(raw); err != nil {
		t.Fatalf("play_log-only rebuild must succeed: %v", err)
	}
	// The relaxed play_log CHECK now accepts a youtube row.
	if _, err := raw.ExecContext(ctx,
		`INSERT INTO play_log (id, track_id, source, played_at)
		 VALUES ('p','t','youtube','2026-07-24T00:00:00Z')`); err != nil {
		t.Fatalf("play_log must accept youtube after rebuild: %v", err)
	}
}

// Open on a real file path exercises the MkdirAll + file-DSN branch (tests otherwise
// use :memory:), and a second Open on the same file must be a clean idempotent no-op
// (all migrations skip).
func TestOpenFilePathAndReopen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "library.db")
	repo, closeFn, err := Open(path)
	if err != nil {
		t.Fatalf("Open file: %v", err)
	}
	// upsert a youtube row to prove the fresh-file schema already admits 'youtube'.
	yt, err := library.NewTrack("yt1", shared.SourceYouTube, "Vid", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	yt.YouTubeURI = "https://youtu.be/x"
	if err := repo.Upsert(context.Background(), yt); err != nil {
		t.Fatalf("upsert youtube on fresh file: %v", err)
	}
	closeFn()

	// reopen the same file: every migration must detect the up-to-date schema and skip.
	repo2, closeFn2, err := Open(path)
	if err != nil {
		t.Fatalf("reopen file: %v", err)
	}
	defer closeFn2()
	if _, err := repo2.Get(context.Background(), "yt1"); err != nil {
		t.Fatalf("row lost across reopen: %v", err)
	}
}

// UserRepo query paths surface the db error on a closed handle (List/Count/Delete).
func TestUserRepoClosedDB(t *testing.T) {
	repo, closeFn, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	users := repo.Users()
	closeFn()
	ctx := context.Background()
	if _, err := users.List(ctx); err == nil {
		t.Fatal("expected List error on closed db")
	}
	if _, err := users.Count(ctx); err == nil {
		t.Fatal("expected Count error on closed db")
	}
	if err := users.Delete(ctx, "x"); err == nil {
		t.Fatal("expected Delete error on closed db")
	}
}

// tableDefLacks reports (false, nil) for a table that does not exist — nothing to
// rebuild. Covers the sql.ErrNoRows branch.
func TestTableDefLacksMissingTable(t *testing.T) {
	raw, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	lacks, err := tableDefLacks(context.Background(), raw, "ghost", "'youtube'")
	if err != nil {
		t.Fatalf("missing table must not error: %v", err)
	}
	if lacks {
		t.Fatal("missing table must report false (nothing to rebuild)")
	}
}

// DeleteAll wipes the catalog and is idempotent (Limpar → start from zero).
func TestDeleteAll(t *testing.T) {
	ctx := context.Background()
	r := openTmp(t)
	if err := r.Upsert(ctx, mk(t, "a")); err != nil {
		t.Fatal(err)
	}
	if err := r.Upsert(ctx, mk(t, "b")); err != nil {
		t.Fatal(err)
	}
	if err := r.DeleteAll(ctx); err != nil {
		t.Fatalf("DeleteAll: %v", err)
	}
	if n, _ := r.Count(ctx); n != 0 {
		t.Fatalf("catalog must be empty after DeleteAll, got %d", n)
	}
	// Idempotent on an already-empty catalog.
	if err := r.DeleteAll(ctx); err != nil {
		t.Fatalf("second DeleteAll: %v", err)
	}
}

// DeleteAll on a closed DB surfaces the error.
func TestDeleteAllClosedDB(t *testing.T) {
	r, closeFn, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	_ = closeFn()
	if err := r.DeleteAll(context.Background()); err == nil {
		t.Fatal("DeleteAll on closed db must error")
	}
}
