package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"autodj/domain/operatorprofile"
	"autodj/ports/outbound"
)

// ProfileRepo implements outbound.ProfileRepo on the profiles table (user-id
// keyed). It shares the TrackRepo *sql.DB handle so an operator's layout lives in
// the same per-user file as the library. Theme/density are columns; the ordered
// panel prefs are a JSON blob. The domain owns validation/normalization (Story 7.4).
type ProfileRepo struct {
	db *sql.DB
}

// Profiles returns a ProfileRepo backed by the same database as this TrackRepo.
func (r *TrackRepo) Profiles() *ProfileRepo {
	return &ProfileRepo{db: r.db}
}

// Get returns the profile for userID, or outbound.ErrProfileNotFound.
func (r *ProfileRepo) Get(ctx context.Context, userID string) (operatorprofile.Profile, error) {
	var theme, density, data string
	var touch int
	var screens int
	err := r.db.QueryRowContext(ctx,
		`SELECT theme, density, touch, screens, data FROM profiles WHERE user_id=?`, userID).
		Scan(&theme, &density, &touch, &screens, &data)
	if errors.Is(err, sql.ErrNoRows) {
		return operatorprofile.Profile{}, outbound.ErrProfileNotFound
	}
	if err != nil {
		return operatorprofile.Profile{}, fmt.Errorf("sqlite: get profile: %w", err)
	}
	var panels []operatorprofile.PanelPref
	if err := json.Unmarshal([]byte(data), &panels); err != nil {
		return operatorprofile.Profile{}, fmt.Errorf("sqlite: decode profile panels: %w", err)
	}
	return operatorprofile.Profile{
		UserID:  userID,
		Theme:   operatorprofile.Theme(theme),
		Density: operatorprofile.Density(density),
		Touch:   touch != 0,
		Screens: screens,
		Panels:  panels,
	}, nil
}

// Upsert inserts or replaces the profile keyed by its UserID.
func (r *ProfileRepo) Upsert(ctx context.Context, p operatorprofile.Profile) error {
	data, err := json.Marshal(p.Panels)
	if err != nil {
		return fmt.Errorf("sqlite: encode profile panels: %w", err)
	}
	touch := 0
	if p.Touch {
		touch = 1
	}
	_, err = r.db.ExecContext(ctx,
		`INSERT INTO profiles (user_id, theme, density, touch, screens, data) VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(user_id) DO UPDATE SET theme=excluded.theme, density=excluded.density, touch=excluded.touch, screens=excluded.screens, data=excluded.data`,
		p.UserID, string(p.Theme), string(p.Density), touch, p.Screens, string(data))
	if err != nil {
		return fmt.Errorf("sqlite: upsert profile: %w", err)
	}
	return nil
}

// compile-time: ProfileRepo satisfies the outbound port.
var _ outbound.ProfileRepo = (*ProfileRepo)(nil)
