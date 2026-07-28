package app

import (
	"context"
	"sync"

	"autodj/domain/rds"
	"autodj/ports/inbound"
	"autodj/ports/outbound"
)

// RDSService drives the Radio Data System sidecar (Story 7.1). It holds the
// station-fixed identity (PI/PS/PTY) and, when enabled, derives the dynamic
// RadioText from now-playing metadata and pushes the full state to the encoder.
//
// RDS carries *metadata only* — no audio — so RadioText may name a DRM Spotify
// track without breaching the two-engine invariant: no PCM crosses, only text.
//
// Config and last state are held in memory — single operator, home use, like
// AIDJService's flow.
type RDSService struct {
	enc outbound.RDSEncoder // optional; nil → UpdateNowPlaying returns ErrEncoderUnavailable

	mu   sync.Mutex
	cfg  rds.Config
	last rds.State
}

// NewRDSService builds the service starting from rds.DefaultConfig (disabled).
// enc is optional: without it the config still reads/writes but UpdateNowPlaying
// returns ErrEncoderUnavailable.
func NewRDSService(enc outbound.RDSEncoder) *RDSService {
	return &RDSService{enc: enc, cfg: rds.DefaultConfig}
}

// Config reports the current station-fixed RDS identity.
func (s *RDSService) Config(_ context.Context) inbound.RDSConfig {
	s.mu.Lock()
	defer s.mu.Unlock()
	return toDTO(s.cfg)
}

// SetConfig validates the identity through the rds domain and stores it,
// returning the stored value. An invalid identity leaves the store unchanged.
func (s *RDSService) SetConfig(_ context.Context, cfg inbound.RDSConfig) (inbound.RDSConfig, error) {
	dc := fromDTO(cfg)
	if err := dc.Validate(); err != nil {
		return inbound.RDSConfig{}, err
	}
	s.mu.Lock()
	s.cfg = dc
	s.mu.Unlock()
	return toDTO(dc), nil
}

// UpdateNowPlaying rebuilds the RDS state from the metadata and pushes it to the
// encoder, returning the pushed state. Returns rds.ErrDisabled when RDS is off,
// an rds validation error for a bad identity, or ErrEncoderUnavailable when no
// encoder is wired.
func (s *RDSService) UpdateNowPlaying(ctx context.Context, title, artist string) (rds.State, error) {
	s.mu.Lock()
	cfg := s.cfg
	s.mu.Unlock()

	state, err := rds.NewState(cfg, title, artist)
	if err != nil {
		return rds.State{}, err
	}
	if s.enc == nil {
		return rds.State{}, inbound.ErrEncoderUnavailable
	}
	if err := s.enc.Push(ctx, state); err != nil {
		return rds.State{}, err
	}
	s.mu.Lock()
	s.last = state
	s.mu.Unlock()
	return state, nil
}

// Current returns the last state pushed (zero State before the first push).
func (s *RDSService) Current(_ context.Context) rds.State {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.last
}

func toDTO(c rds.Config) inbound.RDSConfig {
	return inbound.RDSConfig{Enabled: c.Enabled, PI: c.PI, PS: c.PS, PTY: c.PTY}
}

func fromDTO(c inbound.RDSConfig) rds.Config {
	return rds.Config{Enabled: c.Enabled, PI: c.PI, PS: c.PS, PTY: c.PTY}
}
