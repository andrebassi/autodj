// Package sqlite implements TrackRepo on a single per-user SQLite file via
// modernc.org/sqlite (pure-Go, cgo-free). This is the default local persistence:
// the home/personal library survives restarts without running any DB daemon.
// Same outbound.TrackRepo interface as memory/postgres, so cmd wires whichever
// the environment selects.
package sqlite

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"autodj/domain/library"
	"autodj/domain/shared"
)

//go:embed schema.sql
var schema string

// TrackRepo implements outbound.TrackRepo on SQLite.
type TrackRepo struct {
	db *sql.DB
}

// Open opens (creating if needed) the SQLite database at path, enables WAL and
// foreign keys, applies the embedded schema, and returns the repo plus a close
// func. Use ":memory:" for an ephemeral database (tests). Parent directories are
// created as needed (e.g. ~/.autodj/).
func Open(path string) (*TrackRepo, func() error, error) {
	if path != ":memory:" && path != "" {
		if dir := filepath.Dir(path); dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return nil, nil, fmt.Errorf("sqlite: mkdir %q: %w", dir, err)
			}
		}
	}
	dsn := path + "?_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, nil, fmt.Errorf("sqlite: open: %w", err)
	}
	// A single connection avoids "database is locked" on the shared in-memory DB
	// and keeps WAL writers serialized for the single-user file case.
	db.SetMaxOpenConns(1)
	if _, err := db.ExecContext(context.Background(), schema); err != nil {
		db.Close()
		return nil, nil, fmt.Errorf("sqlite: schema: %w", err)
	}
	/* MIGRAÇÕES IDEMPOTENTES, EM ORDEM.

	   Banco criado antes de uma coluna não a ganha pelo `CREATE TABLE IF NOT
	   EXISTS` — cada uma entra por ALTER. Sem isso o Upsert perde o campo em
	   silêncio, que é a pior forma de perder dado: a pasta some, a medida some, e
	   o backfill reprocessa a mesma faixa em todo boot.

	   A LISTA É ORDENADA e a ordem importa: `ensureSourceCheck` reconstrói a
	   tabela copiando coluna por coluna, então tudo que é ALTER precisa já existir
	   quando ele roda — por isso ele é o último.

	   Era uma escada de sete blocos `if err != nil { db.Close(); return ... }`
	   idênticos. Colapsar num laço não é só estética: cada bloco repetido era um
	   par de linhas que nenhum teste alcança, e o gate de cobertura pagava por
	   elas. */
	migrations := []struct {
		name string
		fn   func(*sql.DB) error
	}{
		{"markers", ensureMarkersColumn},
		{"folders", ensureFoldersColumn},
		{"artwork_url", ensureArtworkColumn},
		{"youtube_uri", ensureYouTubeColumn},
		{"loudness_lufs", ensureLoudnessColumn},
		{"instrumental_path", ensureInstrumentalColumn},
		{"created_at", ensureCreatedAtColumn},
		// POR ÚLTIMO: relaxa o CHECK de `source` p/ aceitar 'youtube'. O SQLite não
		// tem ALTER de CHECK, então isto RECONSTRÓI a tabela copiando as colunas
		// pelo nome — todas as de cima já têm de existir.
		{"source check", ensureSourceCheck},
	}
	for _, m := range migrations {
		if err := m.fn(db); err != nil {
			db.Close()
			return nil, nil, fmt.Errorf("sqlite: migrate %s: %w", m.name, err)
		}
	}
	return &TrackRepo{db: db}, db.Close, nil
}

// ensureMarkersColumn adds the markers column to a pre-existing tracks table.
// Safe on a fresh DB (the embedded schema already creates it → no-op). It probes
// with a zero-row SELECT: if the column already exists the query succeeds and we
// return; otherwise we ALTER the table to add it.
func ensureMarkersColumn(db *sql.DB) error {
	if _, err := db.ExecContext(context.Background(), `SELECT markers FROM tracks LIMIT 0`); err == nil {
		return nil
	}
	_, err := db.ExecContext(context.Background(),
		`ALTER TABLE tracks ADD COLUMN markers TEXT NOT NULL DEFAULT '[]'`)
	return err
}

// ensureFoldersColumn adiciona a coluna folders (pastas do operador) a um tracks
// pré-existente. Mesmo padrão de sonda do ensureMarkersColumn. Sem isto, o banco de
// quem já usa o AutoDJ perde a pasta a cada Upsert — em silêncio.
func ensureFoldersColumn(db *sql.DB) error {
	if _, err := db.ExecContext(context.Background(), `SELECT folders FROM tracks LIMIT 0`); err == nil {
		return nil
	}
	_, err := db.ExecContext(context.Background(),
		`ALTER TABLE tracks ADD COLUMN folders TEXT NOT NULL DEFAULT '[]'`)
	return err
}

// ensureArtworkColumn adds the artwork_url column to a pre-existing tracks table.
// Safe on a fresh DB (the embedded schema already creates it → no-op). Same probe
// pattern as ensureMarkersColumn.
func ensureArtworkColumn(db *sql.DB) error {
	if _, err := db.ExecContext(context.Background(), `SELECT artwork_url FROM tracks LIMIT 0`); err == nil {
		return nil
	}
	_, err := db.ExecContext(context.Background(),
		`ALTER TABLE tracks ADD COLUMN artwork_url TEXT NOT NULL DEFAULT ''`)
	return err
}

// ensureYouTubeColumn adds the youtube_uri column to a pre-existing tracks table.
// Safe on a fresh DB (the embedded schema already creates it → no-op). Same probe
// pattern as ensureMarkersColumn.
func ensureYouTubeColumn(db *sql.DB) error {
	if _, err := db.ExecContext(context.Background(), `SELECT youtube_uri FROM tracks LIMIT 0`); err == nil {
		return nil
	}
	_, err := db.ExecContext(context.Background(),
		`ALTER TABLE tracks ADD COLUMN youtube_uri TEXT NOT NULL DEFAULT ''`)
	return err
}

// ensureLoudnessColumn adds the loudness_lufs column to a pre-existing tracks
// table. Safe on a fresh DB (the embedded schema already creates it → no-op).
// Same probe pattern as ensureMarkersColumn.
func ensureLoudnessColumn(db *sql.DB) error {
	if _, err := db.ExecContext(context.Background(), `SELECT loudness_lufs FROM tracks LIMIT 0`); err == nil {
		return nil
	}
	_, err := db.ExecContext(context.Background(),
		`ALTER TABLE tracks ADD COLUMN loudness_lufs REAL NOT NULL DEFAULT 0`)
	return err
}

// ensureInstrumentalColumn adds the instrumental_path column to a pre-existing
// tracks table. Safe on a fresh DB (the embedded schema already creates it →
// no-op). Same probe pattern as ensureMarkersColumn.
func ensureInstrumentalColumn(db *sql.DB) error {
	if _, err := db.ExecContext(context.Background(), `SELECT instrumental_path FROM tracks LIMIT 0`); err == nil {
		return nil
	}
	_, err := db.ExecContext(context.Background(),
		`ALTER TABLE tracks ADD COLUMN instrumental_path TEXT NOT NULL DEFAULT ''`)
	return err
}

// ensureCreatedAtColumn adds the created_at column to a pre-existing tracks table.
// Safe on a fresh DB (the embedded schema already creates it → no-op). Same probe
// pattern as ensureArtworkColumn. A literal default is required (ALTER ADD COLUMN
// cannot use the non-constant CURRENT_TIMESTAMP), so migrated rows get the epoch.
func ensureCreatedAtColumn(db *sql.DB) error {
	if _, err := db.ExecContext(context.Background(), `SELECT created_at FROM tracks LIMIT 0`); err == nil {
		return nil
	}
	_, err := db.ExecContext(context.Background(),
		`ALTER TABLE tracks ADD COLUMN created_at TEXT NOT NULL DEFAULT '1970-01-01T00:00:00Z'`)
	return err
}

// ensureSourceCheck relaxes the source CHECK constraints on tracks and play_log to
// admit 'youtube'. SQLite has no ALTER for a CHECK, so tables created before YouTube
// carry the old CHECK that rejects 'youtube' on insert. It rebuilds each table
// (create new → copy → drop → rename) only when the current definition lacks
// 'youtube' — idempotent once migrated, no-op on a fresh DB (the embedded schema
// already includes it). Runs inside a transaction with foreign_keys off.
func ensureSourceCheck(db *sql.DB) error {
	ctx := context.Background()
	// Probe for the QUOTED 'youtube' that only appears inside the source CHECK — a
	// bare "youtube" would also match the youtube_uri column name (added by
	// ensureYouTubeColumn just before this runs), making the rebuild wrongly skip.
	needTracks, err := tableDefLacks(ctx, db, "tracks", "'youtube'")
	if err != nil {
		return err
	}
	needLog, err := tableDefLacks(ctx, db, "play_log", "'youtube'")
	if err != nil {
		return err
	}
	if !needTracks && !needLog {
		return nil
	}
	// foreign_keys must be toggled outside the transaction (SQLite ignores the
	// PRAGMA mid-transaction). No table here has FKs, but the rename dance is the
	// documented-safe form regardless.
	if _, err := db.ExecContext(ctx, `PRAGMA foreign_keys=OFF`); err != nil {
		return err
	}
	defer db.ExecContext(ctx, `PRAGMA foreign_keys=ON`) //nolint:errcheck
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	if needTracks {
		if _, err := tx.ExecContext(ctx, rebuildTracksSQL); err != nil {
			return err
		}
	}
	if needLog {
		if _, err := tx.ExecContext(ctx, rebuildPlayLogSQL); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// tableDefLacks reports whether the stored CREATE TABLE definition for name exists
// and does NOT contain sub. A missing table (fresh DB before schema, impossible
// here since schema ran first) reports false — nothing to rebuild.
func tableDefLacks(ctx context.Context, db *sql.DB, name, sub string) (bool, error) {
	var def string
	err := db.QueryRowContext(ctx,
		`SELECT sql FROM sqlite_master WHERE type='table' AND name=?`, name).Scan(&def)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return !strings.Contains(def, sub), nil
}

// rebuildTracksSQL recreates tracks with the current CHECK set (accepting
// 'youtube'), copying every column by name. Column order in the new table need not
// match the old one; the INSERT names both sides explicitly. Indexes are recreated
// (DROP TABLE removed them). Must stay in sync with schema.sql.
const rebuildTracksSQL = `
CREATE TABLE tracks_new (
    id            TEXT    PRIMARY KEY,
    title         TEXT    NOT NULL DEFAULT '',
    artist        TEXT    NOT NULL DEFAULT '',
    album         TEXT    NOT NULL DEFAULT '',
    category      TEXT    NOT NULL DEFAULT 'music',
    source        TEXT    NOT NULL,
    path          TEXT    NOT NULL DEFAULT '',
    spotify_uri   TEXT    NOT NULL DEFAULT '',
    youtube_uri   TEXT    NOT NULL DEFAULT '',
    duration_ms   INTEGER NOT NULL DEFAULT 0,
    bpm           REAL    NOT NULL DEFAULT 0,
    musical_key   TEXT    NOT NULL DEFAULT '',
    waveform_ref  TEXT    NOT NULL DEFAULT '',
    loudness_lufs REAL    NOT NULL DEFAULT 0,
    instrumental_path TEXT NOT NULL DEFAULT '',
    play_count    INTEGER NOT NULL DEFAULT 0,
    pcm_available INTEGER NOT NULL DEFAULT 0,
    mixable       INTEGER NOT NULL DEFAULT 0,
    markers       TEXT    NOT NULL DEFAULT '[]',
    artwork_url   TEXT    NOT NULL DEFAULT '',
    folders       TEXT    NOT NULL DEFAULT '[]',
    created_at    TEXT    NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT source_valid CHECK (source IN ('local','spotify','line_in','tts','youtube')),
    CONSTRAINT spotify_never_mixable CHECK (NOT (source = 'spotify' AND mixable = 1)),
    CONSTRAINT youtube_never_mixable CHECK (NOT (source = 'youtube' AND mixable = 1)),
    CONSTRAINT category_valid CHECK (category IN
        ('music','jingle','soundtrack','effect','commercial','promo','sponsorship','extra'))
);
INSERT INTO tracks_new
  (id, title, artist, album, category, source, path, spotify_uri, youtube_uri,
   duration_ms, bpm, musical_key, waveform_ref, loudness_lufs, play_count,
   pcm_available, mixable, markers, artwork_url, folders, instrumental_path, created_at)
SELECT
  id, title, artist, album, category, source, path, spotify_uri, youtube_uri,
  duration_ms, bpm, musical_key, waveform_ref, loudness_lufs, play_count,
  pcm_available, mixable, markers, artwork_url, folders, instrumental_path, created_at
FROM tracks;
DROP TABLE tracks;
ALTER TABLE tracks_new RENAME TO tracks;
CREATE INDEX IF NOT EXISTS tracks_artist_title ON tracks (artist, title);
CREATE INDEX IF NOT EXISTS tracks_category     ON tracks (category);
CREATE INDEX IF NOT EXISTS tracks_mixable      ON tracks (mixable);`

// rebuildPlayLogSQL recreates play_log with the relaxed source CHECK. play_log is
// append-only; the rebuild preserves every row. Must stay in sync with schema.sql.
const rebuildPlayLogSQL = `
CREATE TABLE play_log_new (
    id            TEXT    PRIMARY KEY,
    track_id      TEXT    NOT NULL,
    title         TEXT    NOT NULL DEFAULT '',
    artist        TEXT    NOT NULL DEFAULT '',
    source        TEXT    NOT NULL,
    played_at     TEXT    NOT NULL,
    duration_ms   INTEGER NOT NULL DEFAULT 0,
    broadcasted   INTEGER NOT NULL DEFAULT 0,
    ecad_relevant INTEGER NOT NULL DEFAULT 0,

    CONSTRAINT play_log_source_valid CHECK (source IN ('local','spotify','line_in','tts','youtube'))
);
INSERT INTO play_log_new
  (id, track_id, title, artist, source, played_at, duration_ms, broadcasted, ecad_relevant)
SELECT
  id, track_id, title, artist, source, played_at, duration_ms, broadcasted, ecad_relevant
FROM play_log;
DROP TABLE play_log;
ALTER TABLE play_log_new RENAME TO play_log;
CREATE INDEX IF NOT EXISTS play_log_played_at ON play_log (played_at);
CREATE INDEX IF NOT EXISTS play_log_track     ON play_log (track_id);`

// marshalMarkers encodes markers as a JSON array string. A nil/empty slice
// becomes "[]" (never "null") for a stable round-trip. Marshaling a slice of
// plain-field structs cannot fail, so the error is discarded.
func marshalMarkers(ms []library.Marker) string {
	if len(ms) == 0 {
		return "[]"
	}
	b, _ := json.Marshal(ms)
	return string(b)
}

// unmarshalMarkers decodes the JSON array string back into markers.
func unmarshalMarkers(s string) []library.Marker {
	if s == "" || s == "[]" {
		return nil
	}
	var ms []library.Marker
	if err := json.Unmarshal([]byte(s), &ms); err != nil {
		return nil
	}
	return ms
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// Upsert inserts or updates a track by primary key.
func (r *TrackRepo) Upsert(ctx context.Context, t *library.Track) error {
	if err := t.Validate(); err != nil {
		return err
	}
	const q = `
INSERT INTO tracks
  (id, title, artist, album, category, source, path, spotify_uri, youtube_uri,
   duration_ms, bpm, musical_key, waveform_ref, loudness_lufs, play_count,
   pcm_available, mixable, markers, artwork_url, folders, instrumental_path)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(id) DO UPDATE SET
  title=excluded.title, artist=excluded.artist, album=excluded.album,
  category=excluded.category, source=excluded.source, path=excluded.path,
  spotify_uri=excluded.spotify_uri, youtube_uri=excluded.youtube_uri,
  duration_ms=excluded.duration_ms,
  bpm=excluded.bpm, musical_key=excluded.musical_key,
  waveform_ref=excluded.waveform_ref, loudness_lufs=excluded.loudness_lufs,
  play_count=excluded.play_count,
  pcm_available=excluded.pcm_available, mixable=excluded.mixable,
  markers=excluded.markers, artwork_url=excluded.artwork_url,
  folders=excluded.folders, instrumental_path=excluded.instrumental_path`
	_, err := r.db.ExecContext(ctx, q,
		t.ID, t.Title, t.Artist, t.Album, t.Category, string(t.Source),
		t.Path, t.SpotifyURI, t.YouTubeURI, t.Duration.Milliseconds(), t.BPM, t.MusicalKey,
		t.WaveformRef, t.LoudnessLUFS, t.PlayCount, b2i(t.PCMAvailable), b2i(t.Mixable),
		marshalMarkers(t.Markers), t.ArtworkURL, marshalFolders(t.Folders), t.InstrumentalPath)
	return err
}

const selectCols = `
SELECT id, title, artist, album, category, source, path, spotify_uri, youtube_uri,
       duration_ms, bpm, musical_key, waveform_ref, loudness_lufs, play_count,
       pcm_available, mixable, markers, artwork_url, folders, instrumental_path
FROM tracks`

// List returns all tracks ordered by artist/title.
func (r *TrackRepo) List(ctx context.Context) ([]*library.Track, error) {
	rows, err := r.db.QueryContext(ctx, selectCols+` ORDER BY artist, title`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*library.Track
	for rows.Next() {
		t, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// Get returns one track by id. Returns sql.ErrNoRows when absent; the app layer
// maps that to library.ErrTrackNotFound.
func (r *TrackRepo) Get(ctx context.Context, id string) (*library.Track, error) {
	return scan(r.db.QueryRowContext(ctx, selectCols+` WHERE id=?`, id))
}

// Count returns the number of tracks.
func (r *TrackRepo) Count(ctx context.Context) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `SELECT count(*) FROM tracks`).Scan(&n)
	return n, err
}

// Delete removes a track by id. Returns library.ErrTrackNotFound when no row
// matched, so the app layer maps it to 404 uniformly across repos.
func (r *TrackRepo) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM tracks WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return library.ErrTrackNotFound
	}
	return nil
}

// DeleteAll wipes the whole catalog (header "Limpar" → start from zero).
// Idempotent: no rows is not an error.
func (r *TrackRepo) DeleteAll(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM tracks`)
	return err
}

// scanner is satisfied by both *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...any) error
}

func scan(s scanner) (*library.Track, error) {
	var (
		t         library.Track
		src       string
		durMs     int64
		pcm, mixI int
		markers   string
		folders   string
	)
	err := s.Scan(&t.ID, &t.Title, &t.Artist, &t.Album, &t.Category, &src,
		&t.Path, &t.SpotifyURI, &t.YouTubeURI, &durMs, &t.BPM, &t.MusicalKey,
		&t.WaveformRef, &t.LoudnessLUFS, &t.PlayCount, &pcm, &mixI, &markers,
		&t.ArtworkURL, &folders, &t.InstrumentalPath)
	if err != nil {
		return nil, err
	}
	t.Source = shared.SourceKind(src)
	t.Duration = time.Duration(durMs) * time.Millisecond
	t.PCMAvailable = pcm != 0
	t.Mixable = mixI != 0
	t.Markers = unmarshalMarkers(markers)
	t.Folders = unmarshalFolders(folders)
	return &t, nil
}

// marshalFolders / unmarshalFolders espelham os de markers: JSON no lugar de tabela
// de ligação. São poucas pastas por faixa e a biblioteca é caseira — uma tabela
// N:N cobraria join em toda leitura pra resolver um problema que não existe nesta
// escala. JSON inválido lê como "sem pasta" em vez de derrubar a listagem inteira.
func marshalFolders(fs []string) string {
	if len(fs) == 0 {
		return "[]"
	}
	b, _ := json.Marshal(fs)
	return string(b)
}

func unmarshalFolders(s string) []string {
	if s == "" || s == "[]" {
		return nil
	}
	var fs []string
	if err := json.Unmarshal([]byte(s), &fs); err != nil {
		return nil
	}
	return fs
}
