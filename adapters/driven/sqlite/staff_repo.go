package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"autodj/domain/staffschedule"
	"autodj/ports/outbound"
)

// StaffRepo implements outbound.StaffScheduleRepo on the staff_shifts table
// (id-keyed, the shift stored as a JSON blob, announcer_id/weekday denormalized).
// It shares the TrackRepo *sql.DB handle so the whole local library lives in one
// file.
type StaffRepo struct {
	db *sql.DB
}

// Staff returns a StaffRepo backed by the same database as this TrackRepo.
func (r *TrackRepo) Staff() *StaffRepo {
	return &StaffRepo{db: r.db}
}

// Save upserts the shift by id.
func (r *StaffRepo) Save(ctx context.Context, s staffschedule.Shift) error {
	b, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("sqlite: encode shift: %w", err)
	}
	_, err = r.db.ExecContext(ctx,
		`INSERT INTO staff_shifts (id, announcer_id, weekday, data) VALUES (?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET announcer_id=excluded.announcer_id, weekday=excluded.weekday, data=excluded.data`,
		s.ID, s.AnnouncerID, int(s.Weekday), string(b))
	if err != nil {
		return fmt.Errorf("sqlite: save shift: %w", err)
	}
	return nil
}

// Get returns the shift with id, or outbound.ErrShiftNotFound.
func (r *StaffRepo) Get(ctx context.Context, id string) (staffschedule.Shift, error) {
	row := r.db.QueryRowContext(ctx, `SELECT data FROM staff_shifts WHERE id=?`, id)
	s, err := scanShift(row)
	if errors.Is(err, sql.ErrNoRows) {
		return staffschedule.Shift{}, outbound.ErrShiftNotFound
	}
	return s, err
}

// Delete removes the shift by id, or outbound.ErrShiftNotFound if unknown.
func (r *StaffRepo) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM staff_shifts WHERE id=?`, id)
	if err != nil {
		return fmt.Errorf("sqlite: delete shift: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite: delete shift: %w", err)
	}
	if n == 0 {
		return outbound.ErrShiftNotFound
	}
	return nil
}

// List returns all shifts ordered by (weekday, id).
func (r *StaffRepo) List(ctx context.Context) ([]staffschedule.Shift, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT data FROM staff_shifts ORDER BY weekday, id`)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list shifts: %w", err)
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
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: list shifts: %w", err)
	}
	return out, nil
}

func scanShift(s rowScanner) (staffschedule.Shift, error) {
	var data string
	if err := s.Scan(&data); err != nil {
		return staffschedule.Shift{}, err
	}
	var shift staffschedule.Shift
	if err := json.Unmarshal([]byte(data), &shift); err != nil {
		return staffschedule.Shift{}, fmt.Errorf("sqlite: decode shift: %w", err)
	}
	return shift, nil
}
