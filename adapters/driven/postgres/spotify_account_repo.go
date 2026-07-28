package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"autodj/domain/spotify"
)

// SpotifyAccountRepo implements outbound.SpotifyAccountRepo on the single-row
// spotify_account table (id=1, JSONB blob). It shares the TrackRepo pool.
type SpotifyAccountRepo struct {
	pool *pgxpool.Pool
}

// SpotifyAccount returns a SpotifyAccountRepo backed by the same pool as this
// TrackRepo.
func (r *TrackRepo) SpotifyAccount() *SpotifyAccountRepo {
	return &SpotifyAccountRepo{pool: r.pool}
}

// Load returns the linked account, or spotify.ErrNoAccount when no row exists.
func (r *SpotifyAccountRepo) Load(ctx context.Context) (spotify.Account, error) {
	var data []byte
	err := r.pool.QueryRow(ctx, `SELECT data FROM spotify_account WHERE id=1`).Scan(&data)
	if errors.Is(err, pgx.ErrNoRows) {
		return spotify.Account{}, spotify.ErrNoAccount
	}
	if err != nil {
		return spotify.Account{}, fmt.Errorf("postgres: load spotify account: %w", err)
	}
	var a spotify.Account
	if err := json.Unmarshal(data, &a); err != nil {
		return spotify.Account{}, fmt.Errorf("postgres: decode spotify account: %w", err)
	}
	return a, nil
}

// Save upserts the single spotify_account row.
func (r *SpotifyAccountRepo) Save(ctx context.Context, a spotify.Account) error {
	data, err := json.Marshal(a)
	if err != nil {
		return fmt.Errorf("postgres: encode spotify account: %w", err)
	}
	_, err = r.pool.Exec(ctx,
		`INSERT INTO spotify_account (id, data) VALUES (1, $1)
		 ON CONFLICT (id) DO UPDATE SET data=EXCLUDED.data`, data)
	if err != nil {
		return fmt.Errorf("postgres: save spotify account: %w", err)
	}
	return nil
}

// Delete unlinks the account by dropping the row. Idempotent.
func (r *SpotifyAccountRepo) Delete(ctx context.Context) error {
	if _, err := r.pool.Exec(ctx, `DELETE FROM spotify_account WHERE id=1`); err != nil {
		return fmt.Errorf("postgres: delete spotify account: %w", err)
	}
	return nil
}
