package rds

import (
	"errors"
	"strings"
	"testing"
)

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want error
	}{
		{"ok", Config{PI: 0x1001, PS: "AUTODJ", PTY: 10}, nil},
		{"ok max ps", Config{PI: 1, PS: "12345678", PTY: 0}, nil},
		{"ok max pty", Config{PI: 1, PS: "X", PTY: 31}, nil},
		{"zero pi", Config{PI: 0, PS: "X", PTY: 0}, ErrBadPI},
		{"empty ps", Config{PI: 1, PS: "", PTY: 0}, ErrBadPS},
		{"long ps", Config{PI: 1, PS: "123456789", PTY: 0}, ErrBadPS},
		{"bad pty", Config{PI: 1, PS: "X", PTY: 32}, ErrBadPTY},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.cfg.Validate(); !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
		})
	}
}

func TestStateValidate(t *testing.T) {
	ok := State{PI: 1, PS: "AUTODJ", PTY: 10, RT: "hi"}
	if err := ok.Validate(); err != nil {
		t.Fatalf("valid state: %v", err)
	}
	tests := []struct {
		name string
		s    State
		want error
	}{
		{"zero pi", State{PI: 0, PS: "X"}, ErrBadPI},
		{"empty ps", State{PI: 1, PS: ""}, ErrBadPS},
		{"long ps", State{PI: 1, PS: "123456789"}, ErrBadPS},
		{"bad pty", State{PI: 1, PS: "X", PTY: 99}, ErrBadPTY},
		{"long rt", State{PI: 1, PS: "X", PTY: 0, RT: strings.Repeat("a", 65)}, ErrBadRT},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.s.Validate(); !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
		})
	}
	// RT exactly 64 is valid.
	if err := (State{PI: 1, PS: "X", PTY: 0, RT: strings.Repeat("a", 64)}).Validate(); err != nil {
		t.Fatalf("RT==64 should be valid: %v", err)
	}
}

func TestRadioTextFrom(t *testing.T) {
	tests := []struct {
		title, artist, want string
	}{
		{"Chuva de Neon", "Os Ventania", "♪ Os Ventania - Chuva de Neon"},
		{"Chuva de Neon", "", "♪ Chuva de Neon"},
		{"", "Os Ventania", "♪ Os Ventania"},
		{"", "", "AutoDJ"},
		{"  Espaço  ", "  Trim  ", "♪ Trim - Espaço"},
	}
	for _, tc := range tests {
		if got := RadioTextFrom(tc.title, tc.artist); got != tc.want {
			t.Fatalf("RadioTextFrom(%q,%q)=%q, want %q", tc.title, tc.artist, got, tc.want)
		}
	}
}

func TestRadioTextTruncate(t *testing.T) {
	long := strings.Repeat("A", 200)
	rt := RadioTextFrom(long, long)
	if n := len([]rune(rt)); n != MaxRT {
		t.Fatalf("RadioText should truncate to %d runes, got %d", MaxRT, n)
	}
}

func TestRadioTextTruncateRunes(t *testing.T) {
	// multibyte title must truncate by rune count, not bytes.
	title := strings.Repeat("é", 100)
	rt := RadioTextFrom(title, "")
	if n := len([]rune(rt)); n != MaxRT {
		t.Fatalf("rune truncation wrong: got %d runes", n)
	}
}

func TestNewState(t *testing.T) {
	cfg := Config{Enabled: true, PI: 0x1001, PS: "AUTODJ", PTY: 10}
	s, err := NewState(cfg, "Chuva de Neon", "Os Ventania")
	if err != nil {
		t.Fatal(err)
	}
	if s.PI != cfg.PI || s.PS != cfg.PS || s.PTY != cfg.PTY {
		t.Fatalf("station fields not copied: %+v", s)
	}
	if s.RT != "♪ Os Ventania - Chuva de Neon" {
		t.Fatalf("RT=%q", s.RT)
	}
	if !s.MS {
		t.Fatal("MS should default to music=true")
	}
	if err := s.Validate(); err != nil {
		t.Fatalf("derived state must be valid: %v", err)
	}
}

func TestNewStateDisabled(t *testing.T) {
	cfg := Config{Enabled: false, PI: 1, PS: "X", PTY: 0}
	if _, err := NewState(cfg, "t", "a"); !errors.Is(err, ErrDisabled) {
		t.Fatalf("disabled config should error, got %v", err)
	}
}

func TestNewStateInvalidConfig(t *testing.T) {
	cfg := Config{Enabled: true, PI: 0, PS: "X", PTY: 0}
	if _, err := NewState(cfg, "t", "a"); !errors.Is(err, ErrBadPI) {
		t.Fatalf("invalid config should surface, got %v", err)
	}
}

func TestDefaultConfigValid(t *testing.T) {
	// DefaultConfig is disabled, but its fixed fields must pass validation.
	if err := DefaultConfig.Validate(); err != nil {
		t.Fatalf("DefaultConfig invalid: %v", err)
	}
}
