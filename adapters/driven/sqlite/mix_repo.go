package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"autodj/domain/mixing"
)

// MixRepo implements outbound.MixSettingsRepo on the single-row mix_settings
// table (id=1, JSON blob). It shares the TrackRepo *sql.DB handle so the whole
// local library plus its mixing config live in one file.
type MixRepo struct {
	db *sql.DB
}

// Mixing returns a MixRepo backed by the same database as this TrackRepo.
func (r *TrackRepo) Mixing() *MixRepo {
	return &MixRepo{db: r.db}
}

// Get returns the stored settings, or mixing.Default() when no row exists yet.
func (r *MixRepo) Get(ctx context.Context) (mixing.Settings, error) {
	var data string
	err := r.db.QueryRowContext(ctx, `SELECT data FROM mix_settings WHERE id=1`).Scan(&data)
	if err == sql.ErrNoRows {
		return mixing.Default(), nil
	}
	if err != nil {
		return mixing.Settings{}, fmt.Errorf("sqlite: get mix settings: %w", err)
	}
	// Decodifica SOBRE o default: campo que ainda não existia quando o blob foi
	// gravado (estação antiga) fica com o valor de fábrica em vez de zero. Sem
	// isto, um `bool` novo nasce `false` no acervo já existente — ou seja, a
	// função sobe DESLIGADA justamente para quem já usa o app, e em silêncio.
	// Campo gravado como `false` de propósito continua `false`: o Go serializa
	// todos os campos (sem omitempty), então a chave está no JSON e sobrescreve.
	s := mixing.Default()
	if err := json.Unmarshal([]byte(data), &s); err != nil {
		return mixing.Settings{}, fmt.Errorf("sqlite: decode mix settings: %w", err)
	}
	return s, nil
}

// Save upserts the single settings row.
func (r *MixRepo) Save(ctx context.Context, s mixing.Settings) error {
	b, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("sqlite: encode mix settings: %w", err)
	}
	_, err = r.db.ExecContext(ctx,
		`INSERT INTO mix_settings (id, data) VALUES (1, ?)
		 ON CONFLICT(id) DO UPDATE SET data=excluded.data`, string(b))
	if err != nil {
		return fmt.Errorf("sqlite: save mix settings: %w", err)
	}
	return nil
}
