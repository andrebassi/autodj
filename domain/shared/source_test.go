package shared

import "testing"

func TestSourceKindValid(t *testing.T) {
	cases := []struct {
		s    SourceKind
		want bool
	}{
		{SourceLocal, true},
		{SourceSpotify, true},
		{SourceLineIn, true},
		{SourceTTS, true},
		{SourceKind("bogus"), false},
		{SourceKind(""), false},
	}
	for _, c := range cases {
		if got := c.s.Valid(); got != c.want {
			t.Errorf("%q.Valid()=%v want %v", c.s, got, c.want)
		}
	}
}

func TestCapabilitiesMatrix(t *testing.T) {
	// The invariant that defines the product: Spotify is DRM, so no PCM and
	// therefore never mixable. Local is fully capable.
	if Capabilities(SourceSpotify).PCM {
		t.Fatal("spotify must never expose PCM")
	}
	if Capabilities(SourceSpotify).Mixable {
		t.Fatal("spotify must never be mixable (ToS + DRM)")
	}
	if Capabilities(SourceSpotify).Beatmatch {
		t.Fatal("spotify cannot beatmatch")
	}
	loc := Capabilities(SourceLocal)
	if !(loc.PCM && loc.Crossfade && loc.Beatmatch && loc.Waveform && loc.Mixable && loc.ECAD) {
		t.Fatalf("local must be fully capable, got %+v", loc)
	}
	// line_in: PCM + mixable but no beatmatch, not ECAD-relevant.
	li := Capabilities(SourceLineIn)
	if !li.PCM || !li.Mixable || li.Beatmatch || li.ECAD {
		t.Fatalf("line_in caps wrong: %+v", li)
	}
	// tts: PCM + mixable, no waveform/ecad.
	tts := Capabilities(SourceTTS)
	if !tts.PCM || !tts.Mixable || tts.Waveform || tts.ECAD {
		t.Fatalf("tts caps wrong: %+v", tts)
	}
	// Unknown kind → zero value (all false), the safe default.
	if Capabilities(SourceKind("nope")) != (Capability{}) {
		t.Fatal("unknown kind must yield zero Capability")
	}
}

func TestAllowedTransitions(t *testing.T) {
	// Spotify: only fade_tts and hard_cut, never crossfade.
	if TransitionAllowed(SourceSpotify, TransitionCrossfade) {
		t.Fatal("spotify must not allow crossfade")
	}
	if !TransitionAllowed(SourceSpotify, TransitionFadeTTS) {
		t.Fatal("spotify should allow fade_tts")
	}
	if !TransitionAllowed(SourceSpotify, TransitionHardCut) {
		t.Fatal("spotify should allow hard_cut")
	}
	if len(AllowedTransitions(SourceSpotify)) != 2 {
		t.Fatal("spotify should have exactly 2 transitions")
	}
	// Local: all three.
	if !TransitionAllowed(SourceLocal, TransitionCrossfade) {
		t.Fatal("local should allow crossfade")
	}
	if len(AllowedTransitions(SourceLocal)) != 3 {
		t.Fatal("local should have 3 transitions")
	}
	if TransitionAllowed(SourceLocal, Transition("weird")) {
		t.Fatal("unknown transition must not be allowed")
	}
}

func TestDefaultTransition(t *testing.T) {
	// PCM-mixable sources enter crossfaded (Liquidsoap overlaps the streams).
	for _, s := range []SourceKind{SourceLocal, SourceLineIn, SourceTTS} {
		if got := DefaultTransition(s); got != TransitionCrossfade {
			t.Fatalf("DefaultTransition(%s) = %q, want crossfade", s, got)
		}
	}
	// Spotify is non-mixable: it plays out-of-band (browser SDK) and can only be
	// hard-cut, never crossfaded into the PCM bus.
	if got := DefaultTransition(SourceSpotify); got != TransitionHardCut {
		t.Fatalf("DefaultTransition(spotify) = %q, want hard_cut", got)
	}
}
