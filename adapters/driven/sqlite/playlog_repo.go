package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"autodj/domain/playlog"
	"autodj/domain/shared"
	"autodj/ports/outbound"
)

// PlayLogRepo implements outbound.PlayLogRepo on the append-only play_log table.
// It shares the TrackRepo *sql.DB handle so history lives in the same per-user
// file as the library. There is no Update/Delete: history is immutable (Story
// 7.5). played_at is stored as UTC RFC3339Nano so lexicographic range queries
// are chronological. Append is idempotent via the deterministic id primary key.
type PlayLogRepo struct {
	db *sql.DB
}

// PlayLog returns a PlayLogRepo backed by the same database as this TrackRepo.
func (r *TrackRepo) PlayLog() *PlayLogRepo {
	return &PlayLogRepo{db: r.db}
}

const tsLayout = time.RFC3339Nano

// Append writes one entry. Idempotent on Entry.ID (ON CONFLICT DO NOTHING).
func (r *PlayLogRepo) Append(ctx context.Context, e playlog.Entry) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO play_log
		   (id, track_id, title, artist, source, played_at, duration_ms, broadcasted, ecad_relevant)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO NOTHING`,
		e.ID, e.TrackID, e.Title, e.Artist, string(e.Source),
		e.PlayedAt.UTC().Format(tsLayout), e.DurationMs,
		boolToInt(e.Broadcasted), boolToInt(e.ECADRelevant))
	if err != nil {
		return fmt.Errorf("sqlite: append play_log: %w", err)
	}
	return nil
}

// Since returns every entry with played_at in [from, to), oldest first.
func (r *PlayLogRepo) Since(ctx context.Context, from, to time.Time) ([]playlog.Entry, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, track_id, title, artist, source, played_at, duration_ms, broadcasted, ecad_relevant
		   FROM play_log
		  WHERE played_at >= ? AND played_at < ?
		  ORDER BY played_at ASC`,
		from.UTC().Format(tsLayout), to.UTC().Format(tsLayout))
	if err != nil {
		return nil, fmt.Errorf("sqlite: since play_log: %w", err)
	}
	defer rows.Close()
	var out []playlog.Entry
	for rows.Next() {
		var e playlog.Entry
		var src, at string
		var broad, ecad int
		if err := rows.Scan(&e.ID, &e.TrackID, &e.Title, &e.Artist, &src, &at, &e.DurationMs, &broad, &ecad); err != nil {
			return nil, fmt.Errorf("sqlite: scan play_log: %w", err)
		}
		e.Source = shared.SourceKind(src)
		e.PlayedAt = parseTS(at)
		e.Broadcasted = broad != 0
		e.ECADRelevant = ecad != 0
		out = append(out, e)
	}
	return out, rows.Err()
}

// Top returns the n most played tracks with played_at in [from, to), ordered by
// count descending. n <= 0 yields nil.
func (r *PlayLogRepo) Top(ctx context.Context, from, to time.Time, n int) ([]playlog.TopItem, error) {
	if n <= 0 {
		return nil, nil
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT track_id, MIN(title), MIN(artist), COUNT(*) AS c
		   FROM play_log
		  WHERE played_at >= ? AND played_at < ?
		  GROUP BY track_id
		  ORDER BY c DESC, MIN(played_at) ASC
		  LIMIT ?`,
		from.UTC().Format(tsLayout), to.UTC().Format(tsLayout), n)
	if err != nil {
		return nil, fmt.Errorf("sqlite: top play_log: %w", err)
	}
	defer rows.Close()
	var out []playlog.TopItem
	for rows.Next() {
		var it playlog.TopItem
		if err := rows.Scan(&it.TrackID, &it.Title, &it.Artist, &it.Count); err != nil {
			return nil, fmt.Errorf("sqlite: scan top: %w", err)
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

func parseTS(s string) time.Time {
	t, _ := time.Parse(tsLayout, s)
	return t
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// compile-time: PlayLogRepo satisfies the outbound port.
var _ outbound.PlayLogRepo = (*PlayLogRepo)(nil)
