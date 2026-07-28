package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"autodj/domain/youtube"
)

// YouTubeAccountRepo implements outbound.YouTubeAccountRepo on the single-row
// youtube_account table (id=1, JSON blob). It shares the TrackRepo *sql.DB so the
// operator's linked account lives in the same local library file.
type YouTubeAccountRepo struct {
	db *sql.DB
}

// YouTubeAccount returns a YouTubeAccountRepo backed by the same database as this
// TrackRepo.
func (r *TrackRepo) YouTubeAccount() *YouTubeAccountRepo {
	return &YouTubeAccountRepo{db: r.db}
}

// Load returns the linked account, or youtube.ErrNoAccount when no row exists.
func (r *YouTubeAccountRepo) Load(ctx context.Context) (youtube.Account, error) {
	var data string
	err := r.db.QueryRowContext(ctx, `SELECT data FROM youtube_account WHERE id=1`).Scan(&data)
	if err == sql.ErrNoRows {
		return youtube.Account{}, youtube.ErrNoAccount
	}
	if err != nil {
		return youtube.Account{}, fmt.Errorf("sqlite: load youtube account: %w", err)
	}
	var a youtube.Account
	if err := json.Unmarshal([]byte(data), &a); err != nil {
		return youtube.Account{}, fmt.Errorf("sqlite: decode youtube account: %w", err)
	}
	return a, nil
}

// Save upserts the single youtube_account row.
func (r *YouTubeAccountRepo) Save(ctx context.Context, a youtube.Account) error {
	data, err := json.Marshal(a)
	if err != nil {
		return fmt.Errorf("sqlite: encode youtube account: %w", err)
	}
	_, err = r.db.ExecContext(ctx,
		`INSERT INTO youtube_account (id, data) VALUES (1, ?)
		 ON CONFLICT(id) DO UPDATE SET data=excluded.data`, string(data))
	if err != nil {
		return fmt.Errorf("sqlite: save youtube account: %w", err)
	}
	return nil
}

// Delete unlinks the account by dropping the row. Idempotent.
func (r *YouTubeAccountRepo) Delete(ctx context.Context) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM youtube_account WHERE id=1`); err != nil {
		return fmt.Errorf("sqlite: delete youtube account: %w", err)
	}
	return nil
}
