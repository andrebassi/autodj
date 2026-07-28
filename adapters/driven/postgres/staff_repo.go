package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"autodj/domain/staffschedule"
	"autodj/ports/outbound"
)

// StaffRepo implements outbound.StaffScheduleRepo on the staff_shifts table
// (id-keyed, the shift as a JSONB blob, announcer_id/weekday denormalized). It
// shares the TrackRepo pool.
type StaffRepo struct {
	pool *pgxpool.Pool
}

// Staff returns a StaffRepo backed by the same pool as this TrackRepo.
func (r *TrackRepo) Staff() *StaffRepo {
	return &StaffRepo{pool: r.pool}
}

// Save upserts the shift by id.
func (r *StaffRepo) Save(ctx context.Context, s staffschedule.Shift) error {
	b, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("postgres: encode shift: %w", err)
	}
	_, err = r.pool.Exec(ctx,
		`INSERT INTO staff_shifts (id, announcer_id, weekday, data) VALUES ($1, $2, $3, $4)
		 ON CONFLICT (id) DO UPDATE SET announcer_id=EXCLUDED.announcer_id, weekday=EXCLUDED.weekday, data=EXCLUDED.data`,
		s.ID, s.AnnouncerID, int(s.Weekday), b)
	if err != nil {
		return fmt.Errorf("postgres: save shift: %w", err)
	}
	return nil
}

// Get returns the shift with id, or outbound.ErrShiftNotFound.
func (r *StaffRepo) Get(ctx context.Context, id string) (staffschedule.Shift, error) {
	s, err := scanShift(r.pool.QueryRow(ctx, `SELECT data FROM staff_shifts WHERE id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return staffschedule.Shift{}, outbound.ErrShiftNotFound
	}
	return s, err
}

// Delete removes the shift by id, or outbound.ErrShiftNotFound if unknown.
func (r *StaffRepo) Delete(ctx context.Context, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM staff_shifts WHERE id=$1`, id)
	if err != nil {
		return fmt.Errorf("postgres: delete shift: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return outbound.ErrShiftNotFound
	}
	return nil
}

// List returns all shifts ordered by (weekday, id).
func (r *StaffRepo) List(ctx context.Context) ([]staffschedule.Shift, error) {
	rows, err := r.pool.Query(ctx, `SELECT data FROM staff_shifts ORDER BY weekday, id`)
	if err != nil {
		return nil, fmt.Errorf("postgres: list shifts: %w", err)
	}
	defer rows.Close()
	var out []staffschedule.Shift
	for rows.Next() {
		s, err := scanShift(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func scanShift(s scanner) (staffschedule.Shift, error) {
	var data []byte
	if err := s.Scan(&data); err != nil {
		return staffschedule.Shift{}, err
	}
	var shift staffschedule.Shift
	if err := json.Unmarshal(data, &shift); err != nil {
		return staffschedule.Shift{}, fmt.Errorf("postgres: decode shift: %w", err)
	}
	return shift, nil
}
