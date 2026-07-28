package memory

import (
	"context"
	"sort"
	"sync"

	"autodj/domain/identity"
	"autodj/ports/outbound"
)

// UserRepo implements outbound.UserRepo in memory (id-keyed, name-indexed). Used
// by the ephemeral repo build and unit tests; the station's real accounts live
// in the SQLite repo. Home use keeps this small and global to the station.
type UserRepo struct {
	mu    sync.RWMutex
	users map[string]identity.User
}

// NewUserRepo returns an empty in-memory user repo.
func NewUserRepo() *UserRepo {
	return &UserRepo{users: map[string]identity.User{}}
}

// Get returns the user with id, or outbound.ErrUserNotFound.
func (r *UserRepo) Get(_ context.Context, id string) (identity.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	u, ok := r.users[id]
	if !ok {
		return identity.User{}, outbound.ErrUserNotFound
	}
	return u, nil
}

// GetByName returns the user with a matching name, or outbound.ErrUserNotFound.
func (r *UserRepo) GetByName(_ context.Context, name string) (identity.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, u := range r.users {
		if u.Name == name {
			return u, nil
		}
	}
	return identity.User{}, outbound.ErrUserNotFound
}

// List returns all users ordered by id (stable output).
func (r *UserRepo) List(context.Context) ([]identity.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]identity.User, 0, len(r.users))
	for _, u := range r.users {
		out = append(out, u)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// Count reports how many users exist.
func (r *UserRepo) Count(context.Context) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.users), nil
}

// Upsert inserts or updates the user by id.
func (r *UserRepo) Upsert(_ context.Context, u identity.User) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.users[u.ID] = u
	return nil
}

// Delete removes the user by id, or returns outbound.ErrUserNotFound.
func (r *UserRepo) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.users[id]; !ok {
		return outbound.ErrUserNotFound
	}
	delete(r.users, id)
	return nil
}

// compile-time: UserRepo satisfies the outbound port.
var _ outbound.UserRepo = (*UserRepo)(nil)
