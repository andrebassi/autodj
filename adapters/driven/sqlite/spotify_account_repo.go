package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"autodj/domain/spotify"
)

// SpotifyAccountRepo implements outbound.SpotifyAccountRepo on the single-row
// spotify_account table (id=1, JSON blob). It shares the TrackRepo *sql.DB so the
// operator's linked account lives in the same local library file.
type SpotifyAccountRepo struct {
	db *sql.DB
}

// SpotifyAccount returns a SpotifyAccountRepo backed by the same database as this
// TrackRepo.
func (r *TrackRepo) SpotifyAccount() *SpotifyAccountRepo {
	return &SpotifyAccountRepo{db: r.db}
}

// Load returns the linked account, or spotify.ErrNoAccount when no row exists.
func (r *SpotifyAccountRepo) Load(ctx context.Context) (spotify.Account, error) {
	var data string
	err := r.db.QueryRowContext(ctx, `SELECT data FROM spotify_account WHERE id=1`).Scan(&data)
	if err == sql.ErrNoRows {
		return spotify.Account{}, spotify.ErrNoAccount
	}
	if err != nil {
		return spotify.Account{}, fmt.Errorf("sqlite: load spotify account: %w", err)
	}
	var a spotify.Account
	if err := json.Unmarshal([]byte(data), &a); err != nil {
		return spotify.Account{}, fmt.Errorf("sqlite: decode spotify account: %w", err)
	}
	return a, nil
}

// Save upserts the single spotify_account row.
func (r *SpotifyAccountRepo) Save(ctx context.Context, a spotify.Account) error {
	data, err := json.Marshal(a)
	if err != nil {
		return fmt.Errorf("sqlite: encode spotify account: %w", err)
	}
	_, err = r.db.ExecContext(ctx,
		`INSERT INTO spotify_account (id, data) VALUES (1, ?)
		 ON CONFLICT(id) DO UPDATE SET data=excluded.data`, string(data))
	if err != nil {
		return fmt.Errorf("sqlite: save spotify account: %w", err)
	}
	return nil
}

// Delete unlinks the account by dropping the row. Idempotent.
func (r *SpotifyAccountRepo) Delete(ctx context.Context) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM spotify_account WHERE id=1`); err != nil {
		return fmt.Errorf("sqlite: delete spotify account: %w", err)
	}
	return nil
}
