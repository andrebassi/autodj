package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"autodj/domain/identity"
	"autodj/ports/outbound"
)

// UserRepo implements outbound.UserRepo on the users table (id-keyed, name
// unique). It shares the TrackRepo *sql.DB handle so operator accounts live in
// the same per-user file as the library. The permission matrix stays in
// domain/identity; this only persists id/name/role/pass_hash.
type UserRepo struct {
	db *sql.DB
}

// Users returns a UserRepo backed by the same database as this TrackRepo.
func (r *TrackRepo) Users() *UserRepo {
	return &UserRepo{db: r.db}
}

// Get returns the user with id, or outbound.ErrUserNotFound.
func (r *UserRepo) Get(ctx context.Context, id string) (identity.User, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, name, role, pass_hash FROM users WHERE id=?`, id)
	u, err := scanUser(row)
	if errors.Is(err, sql.ErrNoRows) {
		return identity.User{}, outbound.ErrUserNotFound
	}
	return u, err
}

// GetByName returns the user with a matching name, or outbound.ErrUserNotFound.
func (r *UserRepo) GetByName(ctx context.Context, name string) (identity.User, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, name, role, pass_hash FROM users WHERE name=?`, name)
	u, err := scanUser(row)
	if errors.Is(err, sql.ErrNoRows) {
		return identity.User{}, outbound.ErrUserNotFound
	}
	return u, err
}

// List returns all users ordered by id.
func (r *UserRepo) List(ctx context.Context) ([]identity.User, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, name, role, pass_hash FROM users ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list users: %w", err)
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
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: list users: %w", err)
	}
	return out, nil
}

// Count reports how many users exist.
func (r *UserRepo) Count(ctx context.Context) (int, error) {
	var n int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n); err != nil {
		return 0, fmt.Errorf("sqlite: count users: %w", err)
	}
	return n, nil
}

// Upsert inserts or updates the user by id.
func (r *UserRepo) Upsert(ctx context.Context, u identity.User) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO users (id, name, role, pass_hash) VALUES (?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET name=excluded.name, role=excluded.role, pass_hash=excluded.pass_hash`,
		u.ID, u.Name, string(u.Role), u.PassHash)
	if err != nil {
		return fmt.Errorf("sqlite: upsert user: %w", err)
	}
	return nil
}

// Delete removes the user by id, or returns outbound.ErrUserNotFound.
func (r *UserRepo) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM users WHERE id=?`, id)
	if err != nil {
		return fmt.Errorf("sqlite: delete user: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite: delete user: %w", err)
	}
	if n == 0 {
		return outbound.ErrUserNotFound
	}
	return nil
}

func scanUser(s rowScanner) (identity.User, error) {
	var id, name, role, hash string
	if err := s.Scan(&id, &name, &role, &hash); err != nil {
		return identity.User{}, err
	}
	return identity.User{ID: id, Name: name, Role: identity.Role(role), PassHash: hash}, nil
}

// compile-time: UserRepo satisfies the outbound port.
var _ outbound.UserRepo = (*UserRepo)(nil)
