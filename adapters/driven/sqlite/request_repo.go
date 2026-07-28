package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"autodj/domain/requests"
	"autodj/ports/outbound"
)

// RequestRepo implements outbound.RequestRepo on the requests table (id-keyed,
// the request stored as a JSON blob, status/created_at denormalized). It shares
// the TrackRepo *sql.DB handle so the whole local library lives in one file.
type RequestRepo struct {
	db *sql.DB
}

// Requests returns a RequestRepo backed by the same database as this TrackRepo.
func (r *TrackRepo) Requests() *RequestRepo {
	return &RequestRepo{db: r.db}
}

// Save upserts the request by id.
func (r *RequestRepo) Save(ctx context.Context, req requests.Request) error {
	b, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("sqlite: encode request: %w", err)
	}
	_, err = r.db.ExecContext(ctx,
		`INSERT INTO requests (id, status, created_at, data) VALUES (?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET status=excluded.status, created_at=excluded.created_at, data=excluded.data`,
		req.ID, string(req.Status), req.CreatedAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00"), string(b))
	if err != nil {
		return fmt.Errorf("sqlite: save request: %w", err)
	}
	return nil
}

// Get returns the request with id, or outbound.ErrRequestNotFound.
func (r *RequestRepo) Get(ctx context.Context, id string) (requests.Request, error) {
	row := r.db.QueryRowContext(ctx, `SELECT data FROM requests WHERE id=?`, id)
	req, err := scanRequest(row)
	if errors.Is(err, sql.ErrNoRows) {
		return requests.Request{}, outbound.ErrRequestNotFound
	}
	return req, err
}

// List returns all requests ordered by id.
func (r *RequestRepo) List(ctx context.Context) ([]requests.Request, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT data FROM requests ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list requests: %w", err)
	}
	defer rows.Close()
	var out []requests.Request
	for rows.Next() {
		req, err := scanRequest(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, req)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: list requests: %w", err)
	}
	return out, nil
}

func scanRequest(s rowScanner) (requests.Request, error) {
	var data string
	if err := s.Scan(&data); err != nil {
		return requests.Request{}, err
	}
	var req requests.Request
	if err := json.Unmarshal([]byte(data), &req); err != nil {
		return requests.Request{}, fmt.Errorf("sqlite: decode request: %w", err)
	}
	return req, nil
}
