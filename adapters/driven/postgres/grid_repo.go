package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"autodj/domain/schedule"
)

// GridRepo implements outbound.GridRepo on the single-row grid table (id=1,
// JSONB blob). It shares the TrackRepo pool.
type GridRepo struct {
	pool *pgxpool.Pool
}

// Grid returns a GridRepo backed by the same pool as this TrackRepo.
func (r *TrackRepo) Grid() *GridRepo {
	return &GridRepo{pool: r.pool}
}

// Get returns the stored grid, or an empty Grid when no row exists yet.
func (r *GridRepo) Get(ctx context.Context) (schedule.Grid, error) {
	var data []byte
	err := r.pool.QueryRow(ctx, `SELECT data FROM grid WHERE id=1`).Scan(&data)
	if errors.Is(err, pgx.ErrNoRows) {
		return schedule.Grid{}, nil
	}
	if err != nil {
		return schedule.Grid{}, fmt.Errorf("postgres: get grid: %w", err)
	}
	var g schedule.Grid
	if err := json.Unmarshal(data, &g); err != nil {
		return schedule.Grid{}, fmt.Errorf("postgres: decode grid: %w", err)
	}
	return g, nil
}

// Save upserts the single grid row.
func (r *GridRepo) Save(ctx context.Context, g schedule.Grid) error {
	b, err := json.Marshal(g)
	if err != nil {
		return fmt.Errorf("postgres: encode grid: %w", err)
	}
	_, err = r.pool.Exec(ctx,
		`INSERT INTO grid (id, data) VALUES (1, $1)
		 ON CONFLICT (id) DO UPDATE SET data=EXCLUDED.data`, b)
	if err != nil {
		return fmt.Errorf("postgres: save grid: %w", err)
	}
	return nil
}
