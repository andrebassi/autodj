package app

import (
	"context"

	"autodj/domain/cartwall"
	"autodj/ports/outbound"
)

// CartwallService reads and writes the operator's cartwall through the repo. It
// resolves each button's track capability (via the TrackRepo) so the domain can
// enforce the two-engine invariant's 6th guard: a button may only reference a
// mixable (local PCM) track. An empty stored board reads back as
// cartwall.Default() so the UI always opens on a usable wall.
type CartwallService struct {
	repo   outbound.CartwallRepo
	tracks outbound.TrackRepo
}

// NewCartwallService builds the cartwall service.
func NewCartwallService(repo outbound.CartwallRepo, tracks outbound.TrackRepo) *CartwallService {
	return &CartwallService{repo: repo, tracks: tracks}
}

// Board returns the current board, falling back to the factory default when the
// store is empty (first boot / ephemeral repo).
func (s *CartwallService) Board(ctx context.Context) (cartwall.Board, error) {
	b, err := s.repo.Load(ctx)
	if err != nil {
		return cartwall.Board{}, err
	}
	if len(b.Buttons) == 0 {
		return cartwall.Default(), nil
	}
	return b, nil
}

// SaveBoard validates every button (re-Assigning into a fresh board so hotkey
// uniqueness, label/page and the mixable guard all hold) then persists it. On
// any validation error the store is left untouched.
func (s *CartwallService) SaveBoard(ctx context.Context, in cartwall.Board) (cartwall.Board, error) {
	var b cartwall.Board
	for _, btn := range in.Buttons {
		if err := b.Assign(btn, s.mixable(ctx, btn.TrackID)); err != nil {
			return cartwall.Board{}, err
		}
	}
	if err := s.repo.Save(ctx, b); err != nil {
		return cartwall.Board{}, err
	}
	return s.repo.Load(ctx)
}

// AssignButton upserts one button into the current board and persists it,
// returning the whole updated board. A validation error leaves the store
// untouched.
func (s *CartwallService) AssignButton(ctx context.Context, btn cartwall.CartButton) (cartwall.Board, error) {
	b, err := s.Board(ctx)
	if err != nil {
		return cartwall.Board{}, err
	}
	if err := b.Assign(btn, s.mixable(ctx, btn.TrackID)); err != nil {
		return cartwall.Board{}, err
	}
	if err := s.repo.Save(ctx, b); err != nil {
		return cartwall.Board{}, err
	}
	return b, nil
}

// ClearButton removes a button by id and persists the board.
func (s *CartwallService) ClearButton(ctx context.Context, id string) error {
	b, err := s.Board(ctx)
	if err != nil {
		return err
	}
	if err := b.Clear(id); err != nil {
		return err
	}
	return s.repo.Save(ctx, b)
}

// mixable resolves whether a track id is a mixable (local PCM) track. An empty
// id or an unknown/DRM track is not mixable — Assign then rejects it, so the
// cartwall never fires Spotify audio.
func (s *CartwallService) mixable(ctx context.Context, trackID string) bool {
	if trackID == "" {
		return false
	}
	t, err := s.tracks.Get(ctx, trackID)
	if err != nil || t == nil {
		return false
	}
	return t.Mixable && t.PCMAvailable
}
