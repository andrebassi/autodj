package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"autodj/domain/schedule"
	"autodj/ports/outbound"
)

// ClockRepo implements outbound.ClockRepo on the clocks table (id-keyed, slots
// as a JSONB blob). It shares the TrackRepo pool.
type ClockRepo struct {
	pool *pgxpool.Pool
}

// Clocks returns a ClockRepo backed by the same pool as this TrackRepo.
func (r *TrackRepo) Clocks() *ClockRepo {
	return &ClockRepo{pool: r.pool}
}

// List returns all clocks ordered by id.
func (r *ClockRepo) List(ctx context.Context) ([]schedule.Clock, error) {
	rows, err := r.pool.Query(ctx, `SELECT id, name, data FROM clocks ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("postgres: list clocks: %w", err)
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
	return out, rows.Err()
}

// Get returns the clock with id, or outbound.ErrClockNotFound.
func (r *ClockRepo) Get(ctx context.Context, id string) (schedule.Clock, error) {
	c, err := scanClock(r.pool.QueryRow(ctx, `SELECT id, name, data FROM clocks WHERE id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return schedule.Clock{}, outbound.ErrClockNotFound
	}
	return c, err
}

// Save upserts the clock by id.
func (r *ClockRepo) Save(ctx context.Context, c schedule.Clock) error {
	b, err := json.Marshal(c.Slots)
	if err != nil {
		return fmt.Errorf("postgres: encode clock slots: %w", err)
	}
	_, err = r.pool.Exec(ctx,
		`INSERT INTO clocks (id, name, data) VALUES ($1, $2, $3)
		 ON CONFLICT (id) DO UPDATE SET name=EXCLUDED.name, data=EXCLUDED.data`,
		c.ID, c.Name, b)
	if err != nil {
		return fmt.Errorf("postgres: save clock: %w", err)
	}
	return nil
}

// Delete removes the clock by id, or returns outbound.ErrClockNotFound.
func (r *ClockRepo) Delete(ctx context.Context, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM clocks WHERE id=$1`, id)
	if err != nil {
		return fmt.Errorf("postgres: delete clock: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return outbound.ErrClockNotFound
	}
	return nil
}

func scanClock(s scanner) (schedule.Clock, error) {
	var (
		c    schedule.Clock
		data []byte
	)
	if err := s.Scan(&c.ID, &c.Name, &data); err != nil {
		return schedule.Clock{}, err
	}
	if err := json.Unmarshal(data, &c.Slots); err != nil {
		return schedule.Clock{}, fmt.Errorf("postgres: decode clock slots: %w", err)
	}
	return c, nil
}
