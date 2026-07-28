// Package rds models the Radio Data System state broadcast alongside FM audio:
// the fixed station identity (PI/PS/PTY) plus the dynamic RadioText derived from
// what is playing now. It is pure — no I/O, no deps beyond the standard library.
//
// RDS carries *metadata*, not audio. RadioText may therefore name a Spotify track
// without touching the DRM audio: no PCM is summed or relayed, so the two-engine
// invariant is untouched. Only text crosses. Field limits follow CENELEC EN 50067:
// PS is 8 characters, RadioText is 64, PTY is 5-bit (0–31), PI is a non-zero
// 16-bit code.
package rds

import (
	"errors"
	"strings"
)

// Errors returned when building or configuring RDS state.
var (
	// ErrBadPS rejects a Programme Service name longer than 8 characters.
	ErrBadPS = errors.New("rds: PS must be 1..8 chars")
	// ErrBadRT rejects RadioText longer than 64 characters.
	ErrBadRT = errors.New("rds: RadioText must be <=64 chars")
	// ErrBadPTY rejects a Programme Type outside 0..31.
	ErrBadPTY = errors.New("rds: PTY must be 0..31")
	// ErrBadPI rejects a zero Programme Identification code.
	ErrBadPI = errors.New("rds: PI must be non-zero")
	// ErrDisabled means RDS output is turned off in the config.
	ErrDisabled = errors.New("rds: disabled")
)

// Field limits from the RDS standard.
const (
	// MaxPS is the Programme Service name length (8 chars).
	MaxPS = 8
	// MaxRT is the RadioText length (64 chars).
	MaxRT = 64
	// MaxPTY is the highest Programme Type value (5-bit).
	MaxPTY = 31
)

// Config is the station-fixed RDS identity. PI/PS/PTY don't change per track;
// RadioText does. Enabled gates whether any state is produced/pushed at all.
type Config struct {
	Enabled bool   `json:"enabled"`
	PI      uint16 `json:"pi"`
	PS      string `json:"ps"`
	PTY     uint8  `json:"pty"`
}

// DefaultConfig is a neutral, valid starting config (fictional station id —
// anti-copyright: no real callsign/brand). Disabled until the operator turns it on.
var DefaultConfig = Config{Enabled: false, PI: 0x1001, PS: "AUTODJ", PTY: 10}

// Validate checks the station-fixed fields against the RDS limits.
func (c Config) Validate() error {
	if c.PI == 0 {
		return ErrBadPI
	}
	if n := len(c.PS); n == 0 || n > MaxPS {
		return ErrBadPS
	}
	if c.PTY > MaxPTY {
		return ErrBadPTY
	}
	return nil
}

// State is a full RDS snapshot pushed to the encoder: the station identity plus
// the current RadioText and the standard flags.
type State struct {
	PI  uint16 `json:"pi"`
	PS  string `json:"ps"`
	PTY uint8  `json:"pty"`
	RT  string `json:"rt"`
	TP  bool   `json:"tp"` // Traffic Programme
	TA  bool   `json:"ta"` // Traffic Announcement
	MS  bool   `json:"ms"` // Music/Speech flag (true = music)
}

// Validate checks every field of a State against the RDS limits.
func (s State) Validate() error {
	if s.PI == 0 {
		return ErrBadPI
	}
	if n := len(s.PS); n == 0 || n > MaxPS {
		return ErrBadPS
	}
	if s.PTY > MaxPTY {
		return ErrBadPTY
	}
	if len(s.RT) > MaxRT {
		return ErrBadRT
	}
	return nil
}

// RadioTextFrom builds the RadioText line from now-playing metadata, truncated to
// the 64-char RDS limit. Empty fields fall back to neutral phrasing so the line
// always reads. Deterministic: same input → same output.
func RadioTextFrom(title, artist string) string {
	title = strings.TrimSpace(title)
	artist = strings.TrimSpace(artist)
	var rt string
	switch {
	case title == "" && artist == "":
		rt = "AutoDJ"
	case artist == "":
		rt = "♪ " + title
	case title == "":
		rt = "♪ " + artist
	default:
		rt = "♪ " + artist + " - " + title
	}
	return truncate(rt, MaxRT)
}

// truncate cuts s to at most n runes (RDS counts characters, not bytes).
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

// NewState derives the dynamic RDS state from the station config and the
// now-playing metadata. Enabled=false → ErrDisabled; an invalid config surfaces
// its validation error. RadioText is built (and truncated) from title/artist —
// metadata only, so any source (including Spotify) may appear.
func NewState(cfg Config, title, artist string) (State, error) {
	if !cfg.Enabled {
		return State{}, ErrDisabled
	}
	if err := cfg.Validate(); err != nil {
		return State{}, err
	}
	return State{
		PI:  cfg.PI,
		PS:  cfg.PS,
		PTY: cfg.PTY,
		RT:  RadioTextFrom(title, artist),
		MS:  true,
	}, nil
}
