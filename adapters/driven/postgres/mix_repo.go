package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"autodj/domain/mixing"
)

// MixRepo implements outbound.MixSettingsRepo on the single-row mix_settings
// table (id=1, JSONB blob). It shares the TrackRepo pool.
type MixRepo struct {
	pool *pgxpool.Pool
}

// Mixing returns a MixRepo backed by the same pool as this TrackRepo.
func (r *TrackRepo) Mixing() *MixRepo {
	return &MixRepo{pool: r.pool}
}

// Get returns the stored settings, or mixing.Default() when no row exists yet.
func (r *MixRepo) Get(ctx context.Context) (mixing.Settings, error) {
	var data []byte
	err := r.pool.QueryRow(ctx, `SELECT data FROM mix_settings WHERE id=1`).Scan(&data)
	if errors.Is(err, pgx.ErrNoRows) {
		return mixing.Default(), nil
	}
	if err != nil {
		return mixing.Settings{}, fmt.Errorf("postgres: get mix settings: %w", err)
	}
	// Decodifica SOBRE o default (mesma razão do adapter SQLite): chave ausente no
	// blob antigo = campo que ainda não existia ⇒ valor de fábrica, não zero.
	s := mixing.Default()
	if err := json.Unmarshal(data, &s); err != nil {
		return mixing.Settings{}, fmt.Errorf("postgres: decode mix settings: %w", err)
	}
	return s, nil
}

// Save upserts the single settings row.
func (r *MixRepo) Save(ctx context.Context, s mixing.Settings) error {
	b, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("postgres: encode mix settings: %w", err)
	}
	_, err = r.pool.Exec(ctx,
		`INSERT INTO mix_settings (id, data) VALUES (1, $1)
		 ON CONFLICT (id) DO UPDATE SET data=EXCLUDED.data`, b)
	if err != nil {
		return fmt.Errorf("postgres: save mix settings: %w", err)
	}
	return nil
}
