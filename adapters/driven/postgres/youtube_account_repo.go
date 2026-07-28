package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"autodj/domain/youtube"
)

// YouTubeAccountRepo implements outbound.YouTubeAccountRepo on the single-row
// youtube_account table (id=1, JSONB blob). It shares the TrackRepo pool.
type YouTubeAccountRepo struct {
	pool *pgxpool.Pool
}

// YouTubeAccount returns a YouTubeAccountRepo backed by the same pool as this
// TrackRepo.
func (r *TrackRepo) YouTubeAccount() *YouTubeAccountRepo {
	return &YouTubeAccountRepo{pool: r.pool}
}

// Load returns the linked account, or youtube.ErrNoAccount when no row exists.
func (r *YouTubeAccountRepo) Load(ctx context.Context) (youtube.Account, error) {
	var data []byte
	err := r.pool.QueryRow(ctx, `SELECT data FROM youtube_account WHERE id=1`).Scan(&data)
	if errors.Is(err, pgx.ErrNoRows) {
		return youtube.Account{}, youtube.ErrNoAccount
	}
	if err != nil {
		return youtube.Account{}, fmt.Errorf("postgres: load youtube account: %w", err)
	}
	var a youtube.Account
	if err := json.Unmarshal(data, &a); err != nil {
		return youtube.Account{}, fmt.Errorf("postgres: decode youtube account: %w", err)
	}
	return a, nil
}

// Save upserts the single youtube_account row.
func (r *YouTubeAccountRepo) Save(ctx context.Context, a youtube.Account) error {
	data, err := json.Marshal(a)
	if err != nil {
		return fmt.Errorf("postgres: encode youtube account: %w", err)
	}
	_, err = r.pool.Exec(ctx,
		`INSERT INTO youtube_account (id, data) VALUES (1, $1)
		 ON CONFLICT (id) DO UPDATE SET data=EXCLUDED.data`, data)
	if err != nil {
		return fmt.Errorf("postgres: save youtube account: %w", err)
	}
	return nil
}

// Delete unlinks the account by dropping the row. Idempotent.
func (r *YouTubeAccountRepo) Delete(ctx context.Context) error {
	if _, err := r.pool.Exec(ctx, `DELETE FROM youtube_account WHERE id=1`); err != nil {
		return fmt.Errorf("postgres: delete youtube account: %w", err)
	}
	return nil
}
