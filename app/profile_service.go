package app

import (
	"context"
	"errors"

	"autodj/domain/identity"
	"autodj/domain/operatorprofile"
	"autodj/ports/inbound"
	"autodj/ports/outbound"
)

// ProfileService reads and saves each operator's player layout (Story 7.4). It
// composes one driven port (ProfileRepo). The domain owns validation and the
// default layout; this service only orchestrates persistence and the ownership
// gate: a user may write only their own profile unless they hold users.manage.
type ProfileService struct {
	repo outbound.ProfileRepo
}

// NewProfileService wires the service over the profile repo.
func NewProfileService(repo outbound.ProfileRepo) *ProfileService {
	return &ProfileService{repo: repo}
}

// Get returns userID's saved layout, or operatorprofile.Default(userID) when the
// user has never saved one. Absence is a default, never an error.
func (s *ProfileService) Get(ctx context.Context, userID string) (operatorprofile.Profile, error) {
	p, err := s.repo.Get(ctx, userID)
	if err != nil {
		if errors.Is(err, outbound.ErrProfileNotFound) {
			return operatorprofile.Default(userID), nil
		}
		return operatorprofile.Profile{}, err
	}
	return p, nil
}

// Save validates and persists a profile. The actor must own the profile (same
// UserID) or hold users.manage; otherwise ErrForbidden. Validation normalizes the
// panel order in place, so the stored (and returned) profile always has a clean
// 0..n-1 order.
func (s *ProfileService) Save(ctx context.Context, actor identity.User, p operatorprofile.Profile) (operatorprofile.Profile, error) {
	if actor.ID != p.UserID && !actor.Can(identity.PermUsersManage) {
		return operatorprofile.Profile{}, inbound.ErrForbidden
	}
	if err := p.Validate(); err != nil {
		return operatorprofile.Profile{}, err
	}
	if err := s.repo.Upsert(ctx, p); err != nil {
		return operatorprofile.Profile{}, err
	}
	return p, nil
}

// compile-time: ProfileService satisfies the inbound port.
var _ inbound.ProfileService = (*ProfileService)(nil)
