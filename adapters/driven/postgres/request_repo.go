package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"autodj/domain/requests"
	"autodj/ports/outbound"
)

// RequestRepo implements outbound.RequestRepo on the requests table (id-keyed,
// the request as a JSONB blob, status/created_at denormalized). It shares the
// TrackRepo pool.
type RequestRepo struct {
	pool *pgxpool.Pool
}

// Requests returns a RequestRepo backed by the same pool as this TrackRepo.
func (r *TrackRepo) Requests() *RequestRepo {
	return &RequestRepo{pool: r.pool}
}

// Save upserts the request by id.
func (r *RequestRepo) Save(ctx context.Context, req requests.Request) error {
	b, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("postgres: encode request: %w", err)
	}
	_, err = r.pool.Exec(ctx,
		`INSERT INTO requests (id, status, created_at, data) VALUES ($1, $2, $3, $4)
		 ON CONFLICT (id) DO UPDATE SET status=EXCLUDED.status, created_at=EXCLUDED.created_at, data=EXCLUDED.data`,
		req.ID, string(req.Status), req.CreatedAt, b)
	if err != nil {
		return fmt.Errorf("postgres: save request: %w", err)
	}
	return nil
}

// Get returns the request with id, or outbound.ErrRequestNotFound.
func (r *RequestRepo) Get(ctx context.Context, id string) (requests.Request, error) {
	req, err := scanRequest(r.pool.QueryRow(ctx, `SELECT data FROM requests WHERE id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return requests.Request{}, outbound.ErrRequestNotFound
	}
	return req, err
}

// List returns all requests ordered by id.
func (r *RequestRepo) List(ctx context.Context) ([]requests.Request, error) {
	rows, err := r.pool.Query(ctx, `SELECT data FROM requests ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("postgres: list requests: %w", err)
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
	return out, rows.Err()
}

func scanRequest(s scanner) (requests.Request, error) {
	var data []byte
	if err := s.Scan(&data); err != nil {
		return requests.Request{}, err
	}
	var req requests.Request
	if err := json.Unmarshal(data, &req); err != nil {
		return requests.Request{}, fmt.Errorf("postgres: decode request: %w", err)
	}
	return req, nil
}
