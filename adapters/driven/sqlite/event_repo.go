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

// EventRepo implements outbound.EventRepo on the events table (id-keyed, the
// recurrence stored as a JSON blob, kind denormalized). It shares the TrackRepo
// *sql.DB handle so the whole local library plus its programming live in one file.
type EventRepo struct {
	db *sql.DB
}

// Events returns an EventRepo backed by the same database as this TrackRepo.
func (r *TrackRepo) Events() *EventRepo {
	return &EventRepo{db: r.db}
}

// List returns all events ordered by id.
func (r *EventRepo) List(ctx context.Context) ([]schedule.ScheduledEvent, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT data FROM events ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list events: %w", err)
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
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: list events: %w", err)
	}
	return out, nil
}

// Get returns the event with id, or outbound.ErrEventNotFound.
func (r *EventRepo) Get(ctx context.Context, id string) (schedule.ScheduledEvent, error) {
	row := r.db.QueryRowContext(ctx, `SELECT data FROM events WHERE id=?`, id)
	e, err := scanEvent(row)
	if errors.Is(err, sql.ErrNoRows) {
		return schedule.ScheduledEvent{}, outbound.ErrEventNotFound
	}
	return e, err
}

// Save upserts the event by id.
func (r *EventRepo) Save(ctx context.Context, e schedule.ScheduledEvent) error {
	b, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("sqlite: encode event: %w", err)
	}
	_, err = r.db.ExecContext(ctx,
		`INSERT INTO events (id, kind, data) VALUES (?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET kind=excluded.kind, data=excluded.data`,
		e.ID, string(e.Kind), string(b))
	if err != nil {
		return fmt.Errorf("sqlite: save event: %w", err)
	}
	return nil
}

// Delete removes the event by id, or returns outbound.ErrEventNotFound.
func (r *EventRepo) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM events WHERE id=?`, id)
	if err != nil {
		return fmt.Errorf("sqlite: delete event: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite: delete event: %w", err)
	}
	if n == 0 {
		return outbound.ErrEventNotFound
	}
	return nil
}

func scanEvent(s rowScanner) (schedule.ScheduledEvent, error) {
	var data string
	if err := s.Scan(&data); err != nil {
		return schedule.ScheduledEvent{}, err
	}
	var e schedule.ScheduledEvent
	if err := json.Unmarshal([]byte(data), &e); err != nil {
		return schedule.ScheduledEvent{}, fmt.Errorf("sqlite: decode event: %w", err)
	}
	return e, nil
}
