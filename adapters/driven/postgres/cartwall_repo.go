package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"autodj/domain/cartwall"
)

// CartwallRepo implements outbound.CartwallRepo on the single-row cartwall table
// (id=1, JSONB blob). It shares the TrackRepo pool.
type CartwallRepo struct {
	pool *pgxpool.Pool
}

// Cartwall returns a CartwallRepo backed by the same pool as this TrackRepo.
func (r *TrackRepo) Cartwall() *CartwallRepo {
	return &CartwallRepo{pool: r.pool}
}

// Load returns the stored board, or an empty Board when no row exists yet (the
// service then falls back to cartwall.Default()).
func (r *CartwallRepo) Load(ctx context.Context) (cartwall.Board, error) {
	var data []byte
	err := r.pool.QueryRow(ctx, `SELECT data FROM cartwall WHERE id=1`).Scan(&data)
	if errors.Is(err, pgx.ErrNoRows) {
		return cartwall.Board{}, nil
	}
	if err != nil {
		return cartwall.Board{}, fmt.Errorf("postgres: load cartwall: %w", err)
	}
	var b cartwall.Board
	if err := json.Unmarshal(data, &b); err != nil {
		return cartwall.Board{}, fmt.Errorf("postgres: decode cartwall: %w", err)
	}
	return b, nil
}

// Save upserts the single cartwall row.
func (r *CartwallRepo) Save(ctx context.Context, b cartwall.Board) error {
	data, err := json.Marshal(b)
	if err != nil {
		return fmt.Errorf("postgres: encode cartwall: %w", err)
	}
	_, err = r.pool.Exec(ctx,
		`INSERT INTO cartwall (id, data) VALUES (1, $1)
		 ON CONFLICT (id) DO UPDATE SET data=EXCLUDED.data`, data)
	if err != nil {
		return fmt.Errorf("postgres: save cartwall: %w", err)
	}
	return nil
}
