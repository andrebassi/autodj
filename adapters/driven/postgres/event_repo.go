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

// EventRepo implements outbound.EventRepo on the events table (id-keyed, the
// recurrence as a JSONB blob, kind denormalized). It shares the TrackRepo pool.
type EventRepo struct {
	pool *pgxpool.Pool
}

// Events returns an EventRepo backed by the same pool as this TrackRepo.
func (r *TrackRepo) Events() *EventRepo {
	return &EventRepo{pool: r.pool}
}

// List returns all events ordered by id.
func (r *EventRepo) List(ctx context.Context) ([]schedule.ScheduledEvent, error) {
	rows, err := r.pool.Query(ctx, `SELECT data FROM events ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("postgres: list events: %w", err)
	}
	defer rows.Close()
	var out []schedule.ScheduledEvent
	for rows.Next() {
		e, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// Get returns the event with id, or outbound.ErrEventNotFound.
func (r *EventRepo) Get(ctx context.Context, id string) (schedule.ScheduledEvent, error) {
	e, err := scanEvent(r.pool.QueryRow(ctx, `SELECT data FROM events WHERE id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return schedule.ScheduledEvent{}, outbound.ErrEventNotFound
	}
	return e, err
}

// Save upserts the event by id.
func (r *EventRepo) Save(ctx context.Context, e schedule.ScheduledEvent) error {
	b, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("postgres: encode event: %w", err)
	}
	_, err = r.pool.Exec(ctx,
		`INSERT INTO events (id, kind, data) VALUES ($1, $2, $3)
		 ON CONFLICT (id) DO UPDATE SET kind=EXCLUDED.kind, data=EXCLUDED.data`,
		e.ID, string(e.Kind), b)
	if err != nil {
		return fmt.Errorf("postgres: save event: %w", err)
	}
	return nil
}

// Delete removes the event by id, or returns outbound.ErrEventNotFound.
func (r *EventRepo) Delete(ctx context.Context, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM events WHERE id=$1`, id)
	if err != nil {
		return fmt.Errorf("postgres: delete event: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return outbound.ErrEventNotFound
	}
	return nil
}

func scanEvent(s scanner) (schedule.ScheduledEvent, error) {
	var data []byte
	if err := s.Scan(&data); err != nil {
		return schedule.ScheduledEvent{}, err
	}
	var e schedule.ScheduledEvent
	if err := json.Unmarshal(data, &e); err != nil {
		return schedule.ScheduledEvent{}, fmt.Errorf("postgres: decode event: %w", err)
	}
	return e, nil
}
