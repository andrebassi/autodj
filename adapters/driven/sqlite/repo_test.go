package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"autodj/domain/library"
	"autodj/domain/shared"
)

func openTmp(t *testing.T) *TrackRepo {
	t.Helper()
	r, closeFn, err := Open(filepath.Join(t.TempDir(), "lib.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = closeFn() })
	return r
}

func mk(t *testing.T, id string) *library.Track {
	t.Helper()
	tr, err := library.NewTrack(id, shared.SourceLocal, id, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	tr.Path = "/" + id
	return tr
}

func TestUpsertGetListCount(t *testing.T) {
	ctx := context.Background()
	r := openTmp(t)

	if n, err := r.Count(ctx); err != nil || n != 0 {
		t.Fatalf("empty count: n=%d err=%v", n, err)
	}
	if _, err := r.Get(ctx, "nope"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("want sql.ErrNoRows, got %v", err)
	}

	if err := r.Upsert(ctx, mk(t, "b")); err != nil {
		t.Fatal(err)
	}
	if err := r.Upsert(ctx, mk(t, "a")); err != nil {
		t.Fatal(err)
	}

	got, err := r.Get(ctx, "a")
	if err != nil || got.ID != "a" {
		t.Fatalf("get a: %+v err=%v", got, err)
	}

	list, _ := r.List(ctx)
	if len(list) != 2 || list[0].ID != "a" || list[1].ID != "b" {
		t.Fatalf("list must be sorted by artist/title: %+v", list)
	}
	if n, _ := r.Count(ctx); n != 2 {
		t.Fatal("count 2")
	}

	// upsert same id replaces, not duplicates
	repl := mk(t, "a")
	repl.Title = "changed"
	if err := r.Upsert(ctx, repl); err != nil {
		t.Fatal(err)
	}
	if n, _ := r.Count(ctx); n != 2 {
		t.Fatal("upsert must not duplicate")
	}
	g, _ := r.Get(ctx, "a")
	if g.Title != "changed" {
		t.Fatal("upsert must replace")
	}
}

// full field round-trip through disk — the point of the whole story.
func TestRoundTrip(t *testing.T) {
	ctx := context.Background()
	r := openTmp(t)

	in := mk(t, "x")
	in.Title = "Órbita Silenciosa"
	in.Artist = "Nébula Fictícia"
	in.Album = "Constelações"
	in.Category = "jingle"
	in.Duration = 187_500 * time.Millisecond
	in.BPM = 128.5
	in.MusicalKey = "8A"
	in.WaveformRef = "peaks/x.json"
	in.LoudnessLUFS = -13.4
	in.PlayCount = 42
	_ = in.SetMarkers([]library.Marker{
		{Kind: library.MarkerIntro, PosMs: 8000},
		{Kind: library.MarkerHook, PosMs: 62000, Label: "refrão"},
	})
	if err := r.Upsert(ctx, in); err != nil {
		t.Fatal(err)
	}

	out, err := r.Get(ctx, "x")
	if err != nil {
		t.Fatal(err)
	}
	if out.Title != in.Title || out.Artist != in.Artist || out.Album != in.Album ||
		out.Category != "jingle" || out.Duration != in.Duration || out.BPM != 128.5 ||
		out.MusicalKey != "8A" || out.WaveformRef != in.WaveformRef ||
		out.PlayCount != 42 || out.Source != shared.SourceLocal {
		t.Fatalf("round-trip mismatch: %+v", out)
	}
	if len(out.Markers) != 2 || out.Markers[0].Kind != library.MarkerIntro ||
		out.Markers[1].Kind != library.MarkerHook || out.Markers[1].Label != "refrão" {
		t.Fatalf("markers round-trip mismatch: %+v", out.Markers)
	}
	if !out.PCMAvailable || !out.Mixable {
		t.Fatalf("local track must be PCM/mixable: %+v", out)
	}
	// As MEDIDAS têm de sobreviver ao round-trip. Um campo esquecido no Upsert não
	// dá erro nenhum: o recurso só deixa de funcionar (ganho sem correção, botão
	// VOZ que nunca habilita) e o backfill re-analisa a faixa em todo boot.
	if out.LoudnessLUFS != -13.4 {
		t.Fatalf("medida perdida no round-trip: loudness=%v", out.LoudnessLUFS)
	}
}

func TestDelete(t *testing.T) {
	ctx := context.Background()
	r := openTmp(t)

	if err := r.Delete(ctx, "nope"); !errors.Is(err, library.ErrTrackNotFound) {
		t.Fatalf("delete missing want ErrTrackNotFound, got %v", err)
	}
	_ = r.Upsert(ctx, mk(t, "x"))
	if err := r.Delete(ctx, "x"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Get(ctx, "x"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("track should be gone after delete")
	}
	if err := r.Delete(ctx, "x"); !errors.Is(err, library.ErrTrackNotFound) {
		t.Fatal("second delete want ErrTrackNotFound")
	}
}

func TestUpsertRejectsInvalid(t *testing.T) {
	bad := &library.Track{ID: "", Source: shared.SourceLocal, Duration: time.Minute}
	if err := openTmp(t).Upsert(context.Background(), bad); err == nil {
		t.Fatal("invalid track must be rejected before hitting disk")
	}
}

// The DB CHECK is defense-in-depth for the two-engine invariant: even if a caller
// bypassed the domain and forced mixable on a spotify row, SQLite must reject it.
func TestSpotifyNeverMixableCheck(t *testing.T) {
	ctx := context.Background()
	r := openTmp(t)
	const q = `INSERT INTO tracks (id, source, spotify_uri, duration_ms, mixable)
	           VALUES (?, 'spotify', 'spotify:track:z', 1000, 1)`
	if _, err := r.db.ExecContext(ctx, q, "z"); err == nil {
		t.Fatal("CHECK spotify_never_mixable must reject a mixable spotify row")
	}
}

func TestBadCategoryCheck(t *testing.T) {
	ctx := context.Background()
	r := openTmp(t)
	const q = `INSERT INTO tracks (id, source, path, duration_ms, category)
	           VALUES (?, 'local', '/z', 1000, 'bogus')`
	if _, err := r.db.ExecContext(ctx, q, "z"); err == nil {
		t.Fatal("CHECK category_valid must reject an unknown category")
	}
}

// persistence across Open: write, close, reopen the same file — data survives.
func TestSurvivesReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "persist.db")

	r1, close1, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := r1.Upsert(ctx, mk(t, "keep")); err != nil {
		t.Fatal(err)
	}
	_ = close1()

	r2, close2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer close2()
	if got, err := r2.Get(ctx, "keep"); err != nil || got.ID != "keep" {
		t.Fatalf("track must survive restart: got=%+v err=%v", got, err)
	}
}

// A spotify track round-trips with the derived flags false (pcm=0, mixable=0),
// exercising b2i(false) and confirming DRM stays non-mixable through the file.
func TestSpotifyUpsertNonMixable(t *testing.T) {
	ctx := context.Background()
	r := openTmp(t)

	tr, err := library.NewTrack("s", shared.SourceSpotify, "Faixa DRM", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	tr.SpotifyURI = "spotify:track:abc"
	if err := r.Upsert(ctx, tr); err != nil {
		t.Fatal(err)
	}
	got, err := r.Get(ctx, "s")
	if err != nil {
		t.Fatal(err)
	}
	if got.PCMAvailable || got.Mixable {
		t.Fatalf("spotify must be non-PCM/non-mixable: %+v", got)
	}
	if list, _ := r.List(ctx); len(list) != 1 {
		t.Fatal("list must include the spotify row")
	}
}

// A closed DB makes every query fail, covering the error-return branches.
func TestClosedDBErrors(t *testing.T) {
	ctx := context.Background()
	r, closeFn, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	_ = closeFn()

	if _, err := r.List(ctx); err == nil {
		t.Fatal("List on closed db must error")
	}
	if _, err := r.Count(ctx); err == nil {
		t.Fatal("Count on closed db must error")
	}
	if err := r.Delete(ctx, "x"); err == nil {
		t.Fatal("Delete on closed db must error")
	}
	if err := r.Upsert(ctx, mk(t, "x")); err == nil {
		t.Fatal("Upsert on closed db must error")
	}
}

// Open must fail cleanly when the parent path can't be a directory (a file).
func TestOpenMkdirError(t *testing.T) {
	f := filepath.Join(t.TempDir(), "notadir")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Open(filepath.Join(f, "lib.db")); err == nil {
		t.Fatal("Open under a file path must error")
	}
}

// A DB created before the markers column (old schema) must gain it on Open, and
// existing rows must read back with an empty marker set (default '[]').
func TestEnsureMarkersColumnMigratesOldSchema(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "old.db")

	// Seed a legacy tracks table WITHOUT the markers column.
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	const legacy = `CREATE TABLE tracks (
	  id TEXT PRIMARY KEY, title TEXT NOT NULL DEFAULT '', artist TEXT NOT NULL DEFAULT '',
	  album TEXT NOT NULL DEFAULT '', category TEXT NOT NULL DEFAULT 'music', source TEXT NOT NULL,
	  path TEXT NOT NULL DEFAULT '', spotify_uri TEXT NOT NULL DEFAULT '',
	  duration_ms INTEGER NOT NULL DEFAULT 0, bpm REAL NOT NULL DEFAULT 0,
	  musical_key TEXT NOT NULL DEFAULT '', waveform_ref TEXT NOT NULL DEFAULT '',
	  play_count INTEGER NOT NULL DEFAULT 0, pcm_available INTEGER NOT NULL DEFAULT 0,
	  mixable INTEGER NOT NULL DEFAULT 0);`
	if _, err := raw.ExecContext(ctx, legacy); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.ExecContext(ctx,
		`INSERT INTO tracks (id, source, path, duration_ms, pcm_available, mixable)
		 VALUES ('legacy','local','/l',1000,1,1)`); err != nil {
		t.Fatal(err)
	}
	_ = raw.Close()

	// Open runs ensureMarkersColumn → adds the column.
	r, closeFn, err := Open(path)
	if err != nil {
		t.Fatalf("open must migrate cleanly: %v", err)
	}
	defer closeFn()

	got, err := r.Get(ctx, "legacy")
	if err != nil {
		t.Fatalf("legacy row must read after migration: %v", err)
	}
	if len(got.Markers) != 0 {
		t.Fatalf("migrated row should have no markers, got %+v", got.Markers)
	}
	// And the column now accepts markers on a fresh upsert.
	got.Path = "/l"
	if err := got.SetMarkers([]library.Marker{{Kind: library.MarkerStart, PosMs: 0}}); err != nil {
		t.Fatal(err)
	}
	if err := r.Upsert(ctx, got); err != nil {
		t.Fatal(err)
	}
	back, _ := r.Get(ctx, "legacy")
	if len(back.Markers) != 1 || back.Markers[0].Kind != library.MarkerStart {
		t.Fatalf("markers must persist after migration: %+v", back.Markers)
	}
}

// A row with corrupt JSON in the markers column must read back as no markers
// (best-effort), never fail the whole Get — covers unmarshalMarkers' error path.
func TestUnmarshalMarkersBadJSON(t *testing.T) {
	ctx := context.Background()
	r := openTmp(t)
	if _, err := r.db.ExecContext(ctx,
		`INSERT INTO tracks (id, source, path, duration_ms, pcm_available, mixable, markers)
		 VALUES ('bad','local','/b',1000,1,1,'{not json')`); err != nil {
		t.Fatal(err)
	}
	got, err := r.Get(ctx, "bad")
	if err != nil {
		t.Fatalf("corrupt markers must not fail Get: %v", err)
	}
	if len(got.Markers) != 0 {
		t.Fatalf("corrupt markers should decode to empty, got %+v", got.Markers)
	}
}

// JSON corrompido na coluna `folders` lê como "sem pasta", nunca derruba o Get —
// espelha o TestUnmarshalMarkersBadJSON. Uma faixa com a organização ilegível
// ainda tem de aparecer na biblioteca; some da PASTA, não da lista.
func TestUnmarshalFoldersBadJSON(t *testing.T) {
	ctx := context.Background()
	r := openTmp(t)
	if _, err := r.db.ExecContext(ctx,
		`INSERT INTO tracks (id, source, path, duration_ms, pcm_available, mixable, folders)
		 VALUES ('badf','local','/b',1000,1,1,'{not json')`); err != nil {
		t.Fatal(err)
	}
	got, err := r.Get(ctx, "badf")
	if err != nil {
		t.Fatalf("folders corrompido não pode derrubar o Get: %v", err)
	}
	if len(got.Folders) != 0 {
		t.Fatalf("folders corrompido deveria decodificar vazio, veio %+v", got.Folders)
	}
}

// ensureMarkersColumn must surface its DB errors: a closed DB fails the PRAGMA
// query, and a DB with no tracks table fails the ALTER.
func TestEnsureMarkersColumnErrors(t *testing.T) {
	// No tracks table → PRAGMA returns nothing, ALTER TABLE fails.
	noTable, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := ensureMarkersColumn(noTable); err == nil {
		t.Fatal("ALTER on missing tracks table must error")
	}
	_ = noTable.Close()

	// Closed DB → PRAGMA query fails.
	closed, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	_ = closed.Close()
	if err := ensureMarkersColumn(closed); err == nil {
		t.Fatal("PRAGMA on closed db must error")
	}
}

func TestMemoryDSN(t *testing.T) {
	r, closeFn, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer closeFn()
	if err := r.Upsert(context.Background(), mk(t, "m")); err != nil {
		t.Fatal(err)
	}
	if n, _ := r.Count(context.Background()); n != 1 {
		t.Fatal("in-memory db must work")
	}
}
