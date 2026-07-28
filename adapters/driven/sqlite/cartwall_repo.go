package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"autodj/domain/cartwall"
)

// CartwallRepo implements outbound.CartwallRepo on the single-row cartwall table
// (id=1, JSON blob). It shares the TrackRepo *sql.DB handle so the operator's
// cartwall lives in the same local library file.
type CartwallRepo struct {
	db *sql.DB
}

// Cartwall returns a CartwallRepo backed by the same database as this TrackRepo.
func (r *TrackRepo) Cartwall() *CartwallRepo {
	return &CartwallRepo{db: r.db}
}

// Load returns the stored board, or an empty Board when no row exists yet (the
// service then falls back to cartwall.Default()).
func (r *CartwallRepo) Load(ctx context.Context) (cartwall.Board, error) {
	var data string
	err := r.db.QueryRowContext(ctx, `SELECT data FROM cartwall WHERE id=1`).Scan(&data)
	if err == sql.ErrNoRows {
		return cartwall.Board{}, nil
	}
	if err != nil {
		return cartwall.Board{}, fmt.Errorf("sqlite: load cartwall: %w", err)
	}
	var b cartwall.Board
	if err := json.Unmarshal([]byte(data), &b); err != nil {
		return cartwall.Board{}, fmt.Errorf("sqlite: decode cartwall: %w", err)
	}
	return b, nil
}

// Save upserts the single cartwall row.
func (r *CartwallRepo) Save(ctx context.Context, b cartwall.Board) error {
	data, err := json.Marshal(b)
	if err != nil {
		return fmt.Errorf("sqlite: encode cartwall: %w", err)
	}
	_, err = r.db.ExecContext(ctx,
		`INSERT INTO cartwall (id, data) VALUES (1, ?)
		 ON CONFLICT(id) DO UPDATE SET data=excluded.data`, string(data))
	if err != nil {
		return fmt.Errorf("sqlite: save cartwall: %w", err)
	}
	return nil
}
