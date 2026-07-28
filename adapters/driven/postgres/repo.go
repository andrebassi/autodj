// Package postgres implements TrackRepo on Postgres via pgx. Same interface as
// the memory repo, so cmd wires whichever the environment provides.
package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"autodj/domain/library"
	"autodj/domain/shared"
)

// TrackRepo implements outbound.TrackRepo on Postgres.
type TrackRepo struct {
	pool *pgxpool.Pool
}

// New returns a TrackRepo over an existing pool.
func New(pool *pgxpool.Pool) *TrackRepo { return &TrackRepo{pool: pool} }

// Connect opens a pgx pool.
func Connect(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	return pgxpool.New(ctx, dsn)
}

// Upsert inserts or updates a track by primary key.
func (r *TrackRepo) Upsert(ctx context.Context, t *library.Track) error {
	if err := t.Validate(); err != nil {
		return err
	}
	markers, err := json.Marshal(t.Markers)
	if err != nil || t.Markers == nil {
		markers = []byte("[]")
	}
	const q = `
INSERT INTO tracks
  (id, title, artist, album, category, source, path, spotify_uri, youtube_uri,
   duration_ms, bpm, musical_key, waveform_ref, loudness_lufs, play_count,
   pcm_available, mixable, markers, artwork_url, instrumental_path)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)
ON CONFLICT (id) DO UPDATE SET
  title=EXCLUDED.title, artist=EXCLUDED.artist, album=EXCLUDED.album,
  category=EXCLUDED.category, source=EXCLUDED.source, path=EXCLUDED.path,
  spotify_uri=EXCLUDED.spotify_uri, youtube_uri=EXCLUDED.youtube_uri,
  duration_ms=EXCLUDED.duration_ms,
  bpm=EXCLUDED.bpm, musical_key=EXCLUDED.musical_key,
  waveform_ref=EXCLUDED.waveform_ref, loudness_lufs=EXCLUDED.loudness_lufs,
  pcm_available=EXCLUDED.pcm_available,
  mixable=EXCLUDED.mixable, markers=EXCLUDED.markers,
  artwork_url=EXCLUDED.artwork_url, instrumental_path=EXCLUDED.instrumental_path`
	_, err = r.pool.Exec(ctx, q,
		t.ID, t.Title, t.Artist, t.Album, t.Category, string(t.Source),
		t.Path, t.SpotifyURI, t.YouTubeURI, t.Duration.Milliseconds(), t.BPM, t.MusicalKey,
		t.WaveformRef, t.LoudnessLUFS, t.PlayCount, t.PCMAvailable, t.Mixable,
		markers, t.ArtworkURL, t.InstrumentalPath)
	return err
}

// List returns all tracks ordered by artist/title.
func (r *TrackRepo) List(ctx context.Context) ([]*library.Track, error) {
	const q = `
SELECT id, title, artist, album, category, source, path, spotify_uri, youtube_uri,
       duration_ms, bpm, musical_key, waveform_ref, loudness_lufs, play_count,
       pcm_available, mixable, markers, artwork_url, instrumental_path
FROM tracks ORDER BY artist, title`
	rows, err := r.pool.Query(ctx, q)
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

// Get returns one track by id.
func (r *TrackRepo) Get(ctx context.Context, id string) (*library.Track, error) {
	const q = `
SELECT id, title, artist, album, category, source, path, spotify_uri, youtube_uri,
       duration_ms, bpm, musical_key, waveform_ref, loudness_lufs, play_count,
       pcm_available, mixable, markers, artwork_url, instrumental_path
FROM tracks WHERE id=$1`
	return scan(r.pool.QueryRow(ctx, q, id))
}

// Count returns the number of tracks.
func (r *TrackRepo) Count(ctx context.Context) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx, `SELECT count(*) FROM tracks`).Scan(&n)
	return n, err
}

// Delete removes a track by id. Returns library.ErrTrackNotFound when no row
// matched, so the app layer can map it to 404 uniformly across repos.
func (r *TrackRepo) Delete(ctx context.Context, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM tracks WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return library.ErrTrackNotFound
	}
	return nil
}

// DeleteAll wipes the whole catalog (header "Limpar" → start from zero).
// Idempotent: no rows is not an error.
func (r *TrackRepo) DeleteAll(ctx context.Context) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM tracks`)
	return err
}

// scanner is satisfied by both pgx.Row and pgx.Rows.
type scanner interface {
	Scan(dest ...any) error
}

func scan(s scanner) (*library.Track, error) {
	var (
		t       library.Track
		src     string
		durMs   int64
		markers []byte
	)
	err := s.Scan(&t.ID, &t.Title, &t.Artist, &t.Album, &t.Category, &src,
		&t.Path, &t.SpotifyURI, &t.YouTubeURI, &durMs, &t.BPM, &t.MusicalKey,
		&t.WaveformRef, &t.LoudnessLUFS, &t.PlayCount, &t.PCMAvailable, &t.Mixable,
		&markers, &t.ArtworkURL, &t.InstrumentalPath)
	if err != nil {
		return nil, err
	}
	t.Source = shared.SourceKind(src)
	t.Duration = time.Duration(durMs) * time.Millisecond
	if len(markers) > 0 {
		_ = json.Unmarshal(markers, &t.Markers)
	}
	return &t, nil
}
