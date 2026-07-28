package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"autodj/domain/schedule"
	"autodj/ports/outbound"
)

// ClockRepo implements outbound.ClockRepo on the clocks table (id-keyed, slots
// as a JSON blob). It shares the TrackRepo *sql.DB handle so the whole local
// library plus its programming live in one file.
type ClockRepo struct {
	db *sql.DB
}

// Clocks returns a ClockRepo backed by the same database as this TrackRepo.
func (r *TrackRepo) Clocks() *ClockRepo {
	return &ClockRepo{db: r.db}
}

// List returns all clocks ordered by id.
func (r *ClockRepo) List(ctx context.Context) ([]schedule.Clock, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, name, data FROM clocks ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list clocks: %w", err)
	}
	defer rows.Close()
	var out []schedule.Clock
	for rows.Next() {
		c, err := scanClock(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: list clocks: %w", err)
	}
	return out, nil
}

// Get returns the clock with id, or outbound.ErrClockNotFound.
func (r *ClockRepo) Get(ctx context.Context, id string) (schedule.Clock, error) {
	row := r.db.QueryRowContext(ctx, `SELECT id, name, data FROM clocks WHERE id=?`, id)
	c, err := scanClock(row)
	if errors.Is(err, sql.ErrNoRows) {
		return schedule.Clock{}, outbound.ErrClockNotFound
	}
	return c, err
}

// Save upserts the clock by id.
func (r *ClockRepo) Save(ctx context.Context, c schedule.Clock) error {
	b, err := json.Marshal(c.Slots)
	if err != nil {
		return fmt.Errorf("sqlite: encode clock slots: %w", err)
	}
	_, err = r.db.ExecContext(ctx,
		`INSERT INTO clocks (id, name, data) VALUES (?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET name=excluded.name, data=excluded.data`,
		c.ID, c.Name, string(b))
	if err != nil {
		return fmt.Errorf("sqlite: save clock: %w", err)
	}
	return nil
}

// Delete removes the clock by id, or returns outbound.ErrClockNotFound.
func (r *ClockRepo) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM clocks WHERE id=?`, id)
	if err != nil {
		return fmt.Errorf("sqlite: delete clock: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite: delete clock: %w", err)
	}
	if n == 0 {
		return outbound.ErrClockNotFound
	}
	return nil
}

// rowScanner unifies *sql.Row and *sql.Rows for scanClock.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanClock(s rowScanner) (schedule.Clock, error) {
	var c schedule.Clock
	var data string
	if err := s.Scan(&c.ID, &c.Name, &data); err != nil {
		return schedule.Clock{}, err
	}
	if err := json.Unmarshal([]byte(data), &c.Slots); err != nil {
		return schedule.Clock{}, fmt.Errorf("sqlite: decode clock slots: %w", err)
	}
	return c, nil
}
