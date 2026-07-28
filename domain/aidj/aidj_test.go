package aidj

import (
	"strings"
	"testing"

	"autodj/domain/shared"
)

func TestVoices(t *testing.T) {
	vs := Voices()
	if len(vs) != 3 || vs[0] != DefaultVoice {
		t.Fatalf("Voices() = %v", vs)
	}
}

func TestValidVoice(t *testing.T) {
	cases := []struct {
		v    Voice
		want bool
	}{
		{"", true},
		{DefaultVoice, true},
		{VoiceNimbus, true},
		{VoiceFaber, true},
		{"bogus-voice", false},
	}
	for _, c := range cases {
		if got := ValidVoice(c.v); got != c.want {
			t.Errorf("ValidVoice(%q) = %v, want %v", c.v, got, c.want)
		}
	}
}

func TestScriptRender(t *testing.T) {
	cases := []struct {
		name string
		s    Script
		m    TrackMeta
		want []string // substrings that must be present
	}{
		{
			name: "default template full meta",
			s:    DefaultScript,
			m:    TrackMeta{Title: "Chuva de Neon", Artist: "Os Ventania"},
			want: []string{"Chuva de Neon", "Os Ventania"},
		},
		{
			name: "empty template falls back to default",
			s:    Script{Template: "   "},
			m:    TrackMeta{Title: "Aurora Fria", Artist: "Duo Espelho"},
			want: []string{"Aurora Fria", "Duo Espelho"},
		},
		{
			name: "empty title/artist use neutral phrasing",
			s:    Script{Template: "{title} / {artist}"},
			m:    TrackMeta{},
			want: []string{"a próxima faixa", "um artista da casa"},
		},
		{
			name: "custom template",
			s:    Script{Template: "No ar: {title}!"},
			m:    TrackMeta{Title: "Bloco X", Artist: "irrelevante"},
			want: []string{"No ar: Bloco X!"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := c.s.Render(c.m)
			for _, w := range c.want {
				if !strings.Contains(got, w) {
					t.Errorf("Render() = %q, missing %q", got, w)
				}
			}
		})
	}
}

func TestEstimateDurationMs(t *testing.T) {
	cases := []struct {
		name string
		text string
		want int64
	}{
		{"empty floors to min", "", minDurationMs},
		{"one word floors to min", "olá", minDurationMs},
		{"150 words = 60s", strings.TrimSpace(strings.Repeat("w ", 150)), 60000},
		{"300 words = 120s", strings.TrimSpace(strings.Repeat("w ", 300)), 120000},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := EstimateDurationMs(c.text); got != c.want {
				t.Errorf("EstimateDurationMs(%d words) = %d, want %d",
					len(strings.Fields(c.text)), got, c.want)
			}
		})
	}
}

func TestHash(t *testing.T) {
	base := Hash("A seguir, Chuva de Neon", DefaultVoice)
	if len(base) != 40 {
		t.Fatalf("Hash len = %d, want 40 (sha1 hex)", len(base))
	}
	// deterministic
	if base != Hash("A seguir, Chuva de Neon", DefaultVoice) {
		t.Error("Hash not deterministic")
	}
	// empty voice normalizes to DefaultVoice → same hash
	if base != Hash("A seguir, Chuva de Neon", "") {
		t.Error("empty voice must normalize to DefaultVoice")
	}
	// different text → different hash
	if base == Hash("outro texto", DefaultVoice) {
		t.Error("different text should hash differently")
	}
	// different voice → different hash
	if base == Hash("A seguir, Chuva de Neon", VoiceNimbus) {
		t.Error("different voice should hash differently")
	}
}

func TestNewVoiceover(t *testing.T) {
	t.Run("empty text rejected", func(t *testing.T) {
		if _, err := NewVoiceover("   ", DefaultVoice); err != ErrEmptyText {
			t.Fatalf("err = %v, want ErrEmptyText", err)
		}
	})

	t.Run("builds with hash id and duration", func(t *testing.T) {
		v, err := NewVoiceover("A seguir, Chuva de Neon com Os Ventania.", VoiceNimbus)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if v.ID != v.Hash || v.Hash != Hash(v.Text, VoiceNimbus) {
			t.Errorf("ID/Hash mismatch: id=%s hash=%s", v.ID, v.Hash)
		}
		if v.Voice != VoiceNimbus {
			t.Errorf("Voice = %q", v.Voice)
		}
		if v.DurationMs != EstimateDurationMs(v.Text) {
			t.Errorf("DurationMs = %d", v.DurationMs)
		}
		if v.AudioRef != "" {
			t.Errorf("AudioRef should be empty until synthesis, got %q", v.AudioRef)
		}
	})

	t.Run("empty voice defaults", func(t *testing.T) {
		v, err := NewVoiceover("olá mundo", "")
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if v.Voice != DefaultVoice {
			t.Errorf("Voice = %q, want default", v.Voice)
		}
	})
}

func TestPlanTransition(t *testing.T) {
	cases := []struct {
		name        string
		next        shared.SourceKind
		voiceoverID string
		wantKind    shared.Transition
		wantVoID    string
	}{
		{
			name:        "voiceover present → fade_tts (Spotify next)",
			next:        shared.SourceSpotify,
			voiceoverID: "abc123",
			wantKind:    shared.TransitionFadeTTS,
			wantVoID:    "abc123",
		},
		{
			name:        "voiceover present → fade_tts (PCM next)",
			next:        shared.SourceLocal,
			voiceoverID: "def456",
			wantKind:    shared.TransitionFadeTTS,
			wantVoID:    "def456",
		},
		{
			name:     "no voiceover + Spotify (no PCM) → hard_cut",
			next:     shared.SourceSpotify,
			wantKind: shared.TransitionHardCut,
		},
		{
			name:     "no voiceover + local PCM → crossfade",
			next:     shared.SourceLocal,
			wantKind: shared.TransitionCrossfade,
		},
		{
			name:     "no voiceover + line_in PCM → crossfade",
			next:     shared.SourceLineIn,
			wantKind: shared.TransitionCrossfade,
		},
		{
			name:     "no voiceover + tts PCM → crossfade",
			next:     shared.SourceTTS,
			wantKind: shared.TransitionCrossfade,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := PlanTransition(c.next, c.voiceoverID)
			if p.Kind != c.wantKind {
				t.Errorf("Kind = %q, want %q", p.Kind, c.wantKind)
			}
			if p.VoiceoverID != c.wantVoID {
				t.Errorf("VoiceoverID = %q, want %q", p.VoiceoverID, c.wantVoID)
			}
		})
	}
}
