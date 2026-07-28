package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"autodj/domain/schedule"
)

// GridRepo implements outbound.GridRepo on the single-row grid table (id=1, JSON
// blob). It shares the TrackRepo *sql.DB handle so the whole local library plus
// its programming grid live in one file.
type GridRepo struct {
	db *sql.DB
}

// Grid returns a GridRepo backed by the same database as this TrackRepo.
func (r *TrackRepo) Grid() *GridRepo {
	return &GridRepo{db: r.db}
}

// Get returns the stored grid, or an empty Grid when no row exists yet.
func (r *GridRepo) Get(ctx context.Context) (schedule.Grid, error) {
	var data string
	err := r.db.QueryRowContext(ctx, `SELECT data FROM grid WHERE id=1`).Scan(&data)
	if err == sql.ErrNoRows {
		return schedule.Grid{}, nil
	}
	if err != nil {
		return schedule.Grid{}, fmt.Errorf("sqlite: get grid: %w", err)
	}
	var g schedule.Grid
	if err := json.Unmarshal([]byte(data), &g); err != nil {
		return schedule.Grid{}, fmt.Errorf("sqlite: decode grid: %w", err)
	}
	return g, nil
}

// Save upserts the single grid row.
func (r *GridRepo) Save(ctx context.Context, g schedule.Grid) error {
	b, err := json.Marshal(g)
	if err != nil {
		return fmt.Errorf("sqlite: encode grid: %w", err)
	}
	_, err = r.db.ExecContext(ctx,
		`INSERT INTO grid (id, data) VALUES (1, ?)
		 ON CONFLICT(id) DO UPDATE SET data=excluded.data`, string(b))
	if err != nil {
		return fmt.Errorf("sqlite: save grid: %w", err)
	}
	return nil
}
