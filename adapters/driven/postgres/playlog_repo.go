package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"autodj/domain/playlog"
	"autodj/domain/shared"
	"autodj/ports/outbound"
)

// PlayLogRepo implements outbound.PlayLogRepo on the append-only play_log table.
// It shares the TrackRepo pool. There is no Update/Delete: history is immutable
// (Story 7.5) — the source of truth for ECAD and the irradiation proof. Append
// is idempotent via the deterministic id primary key (ON CONFLICT DO NOTHING).
type PlayLogRepo struct {
	pool *pgxpool.Pool
}

// PlayLog returns a PlayLogRepo backed by the same pool as this TrackRepo.
func (r *TrackRepo) PlayLog() *PlayLogRepo {
	return &PlayLogRepo{pool: r.pool}
}

// Append writes one entry. Idempotent on Entry.ID.
func (r *PlayLogRepo) Append(ctx context.Context, e playlog.Entry) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO play_log
		   (id, track_id, title, artist, source, played_at, duration_ms, broadcasted, ecad_relevant)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 ON CONFLICT (id) DO NOTHING`,
		e.ID, e.TrackID, e.Title, e.Artist, string(e.Source),
		e.PlayedAt.UTC(), e.DurationMs, e.Broadcasted, e.ECADRelevant)
	if err != nil {
		return fmt.Errorf("postgres: append play_log: %w", err)
	}
	return nil
}

// Since returns every entry with played_at in [from, to), oldest first.
func (r *PlayLogRepo) Since(ctx context.Context, from, to time.Time) ([]playlog.Entry, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, track_id, title, artist, source, played_at, duration_ms, broadcasted, ecad_relevant
		   FROM play_log
		  WHERE played_at >= $1 AND played_at < $2
		  ORDER BY played_at ASC`,
		from.UTC(), to.UTC())
	if err != nil {
		return nil, fmt.Errorf("postgres: since play_log: %w", err)
	}
	defer rows.Close()
	var out []playlog.Entry
	for rows.Next() {
		var e playlog.Entry
		var src string
		if err := rows.Scan(&e.ID, &e.TrackID, &e.Title, &e.Artist, &src,
			&e.PlayedAt, &e.DurationMs, &e.Broadcasted, &e.ECADRelevant); err != nil {
			return nil, fmt.Errorf("postgres: scan play_log: %w", err)
		}
		e.Source = shared.SourceKind(src)
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
	rows, err := r.pool.Query(ctx,
		`SELECT track_id, MIN(title), MIN(artist), COUNT(*) AS c
		   FROM play_log
		  WHERE played_at >= $1 AND played_at < $2
		  GROUP BY track_id
		  ORDER BY c DESC, MIN(played_at) ASC
		  LIMIT $3`,
		from.UTC(), to.UTC(), n)
	if err != nil {
		return nil, fmt.Errorf("postgres: top play_log: %w", err)
	}
	defer rows.Close()
	var out []playlog.TopItem
	for rows.Next() {
		var it playlog.TopItem
		if err := rows.Scan(&it.TrackID, &it.Title, &it.Artist, &it.Count); err != nil {
			return nil, fmt.Errorf("postgres: scan top: %w", err)
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// compile-time: PlayLogRepo satisfies the outbound port.
var _ outbound.PlayLogRepo = (*PlayLogRepo)(nil)
