package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"autodj/domain/operatorprofile"
	"autodj/ports/outbound"
)

// ProfileRepo implements outbound.ProfileRepo on the profiles table (user-id
// keyed). It shares the TrackRepo pool. Theme/density are columns; the ordered
// panel prefs are a JSONB blob. The domain owns validation/normalization (Story 7.4).
type ProfileRepo struct {
	pool *pgxpool.Pool
}

// Profiles returns a ProfileRepo backed by the same pool as this TrackRepo.
func (r *TrackRepo) Profiles() *ProfileRepo {
	return &ProfileRepo{pool: r.pool}
}

// Get returns the profile for userID, or outbound.ErrProfileNotFound.
func (r *ProfileRepo) Get(ctx context.Context, userID string) (operatorprofile.Profile, error) {
	var theme, density string
	var touch bool
	var screens int
	var data []byte
	err := r.pool.QueryRow(ctx,
		`SELECT theme, density, touch, screens, data FROM profiles WHERE user_id=$1`, userID).
		Scan(&theme, &density, &touch, &screens, &data)
	if errors.Is(err, pgx.ErrNoRows) {
		return operatorprofile.Profile{}, outbound.ErrProfileNotFound
	}
	if err != nil {
		return operatorprofile.Profile{}, fmt.Errorf("postgres: get profile: %w", err)
	}
	var panels []operatorprofile.PanelPref
	if err := json.Unmarshal(data, &panels); err != nil {
		return operatorprofile.Profile{}, fmt.Errorf("postgres: decode profile panels: %w", err)
	}
	return operatorprofile.Profile{
		UserID:  userID,
		Theme:   operatorprofile.Theme(theme),
		Density: operatorprofile.Density(density),
		Touch:   touch,
		Screens: screens,
		Panels:  panels,
	}, nil
}

// Upsert inserts or replaces the profile keyed by its UserID.
func (r *ProfileRepo) Upsert(ctx context.Context, p operatorprofile.Profile) error {
	data, err := json.Marshal(p.Panels)
	if err != nil {
		return fmt.Errorf("postgres: encode profile panels: %w", err)
	}
	_, err = r.pool.Exec(ctx,
		`INSERT INTO profiles (user_id, theme, density, touch, screens, data) VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (user_id) DO UPDATE SET theme=EXCLUDED.theme, density=EXCLUDED.density, touch=EXCLUDED.touch, screens=EXCLUDED.screens, data=EXCLUDED.data`,
		p.UserID, string(p.Theme), string(p.Density), p.Touch, p.Screens, data)
	if err != nil {
		return fmt.Errorf("postgres: upsert profile: %w", err)
	}
	return nil
}

// compile-time: ProfileRepo satisfies the outbound port.
var _ outbound.ProfileRepo = (*ProfileRepo)(nil)
