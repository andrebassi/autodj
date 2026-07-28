package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"autodj/domain/identity"
	"autodj/ports/outbound"
)

// UserRepo implements outbound.UserRepo on the users table (id-keyed, name
// unique). It shares the TrackRepo pool. The permission matrix stays in
// domain/identity; this only persists id/name/role/pass_hash.
type UserRepo struct {
	pool *pgxpool.Pool
}

// Users returns a UserRepo backed by the same pool as this TrackRepo.
func (r *TrackRepo) Users() *UserRepo {
	return &UserRepo{pool: r.pool}
}

// Get returns the user with id, or outbound.ErrUserNotFound.
func (r *UserRepo) Get(ctx context.Context, id string) (identity.User, error) {
	u, err := scanUser(r.pool.QueryRow(ctx,
		`SELECT id, name, role, pass_hash FROM users WHERE id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return identity.User{}, outbound.ErrUserNotFound
	}
	return u, err
}

// GetByName returns the user with a matching name, or outbound.ErrUserNotFound.
func (r *UserRepo) GetByName(ctx context.Context, name string) (identity.User, error) {
	u, err := scanUser(r.pool.QueryRow(ctx,
		`SELECT id, name, role, pass_hash FROM users WHERE name=$1`, name))
	if errors.Is(err, pgx.ErrNoRows) {
		return identity.User{}, outbound.ErrUserNotFound
	}
	return u, err
}

// List returns all users ordered by id.
func (r *UserRepo) List(ctx context.Context) ([]identity.User, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, name, role, pass_hash FROM users ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("postgres: list users: %w", err)
	}
	defer rows.Close()
	var out []identity.User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// Count reports how many users exist.
func (r *UserRepo) Count(ctx context.Context) (int, error) {
	var n int
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM users`).Scan(&n); err != nil {
		return 0, fmt.Errorf("postgres: count users: %w", err)
	}
	return n, nil
}

// Upsert inserts or updates the user by id.
func (r *UserRepo) Upsert(ctx context.Context, u identity.User) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO users (id, name, role, pass_hash) VALUES ($1, $2, $3, $4)
		 ON CONFLICT (id) DO UPDATE SET name=EXCLUDED.name, role=EXCLUDED.role, pass_hash=EXCLUDED.pass_hash`,
		u.ID, u.Name, string(u.Role), u.PassHash)
	if err != nil {
		return fmt.Errorf("postgres: upsert user: %w", err)
	}
	return nil
}

// Delete removes the user by id, or returns outbound.ErrUserNotFound.
func (r *UserRepo) Delete(ctx context.Context, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, id)
	if err != nil {
		return fmt.Errorf("postgres: delete user: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return outbound.ErrUserNotFound
	}
	return nil
}

func scanUser(s scanner) (identity.User, error) {
	var id, name, role, hash string
	if err := s.Scan(&id, &name, &role, &hash); err != nil {
		return identity.User{}, err
	}
	return identity.User{ID: id, Name: name, Role: identity.Role(role), PassHash: hash}, nil
}

// compile-time: UserRepo satisfies the outbound port.
var _ outbound.UserRepo = (*UserRepo)(nil)
