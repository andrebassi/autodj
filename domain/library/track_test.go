package library

import (
	"testing"
	"time"

	"autodj/domain/shared"
)

func TestNewTrackDerivesFlags(t *testing.T) {
	loc, err := NewTrack("1", shared.SourceLocal, "Song", 3*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if !loc.PCMAvailable || !loc.Mixable {
		t.Fatal("local must be PCM + mixable")
	}

	sp, err := NewTrack("2", shared.SourceSpotify, "Sp", 3*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if sp.PCMAvailable || sp.Mixable {
		t.Fatal("spotify must never be PCM or mixable — invariant")
	}
}

func TestNewTrackErrors(t *testing.T) {
	if _, err := NewTrack("", shared.SourceLocal, "x", time.Minute); err != ErrEmptyID {
		t.Errorf("want ErrEmptyID, got %v", err)
	}
	if _, err := NewTrack("1", shared.SourceKind("z"), "x", time.Minute); err != ErrBadSource {
		t.Errorf("want ErrBadSource, got %v", err)
	}
	if _, err := NewTrack("1", shared.SourceLocal, "x", 0); err != ErrBadDuration {
		t.Errorf("want ErrBadDuration, got %v", err)
	}
}

func TestValidate(t *testing.T) {
	loc := &Track{ID: "1", Source: shared.SourceLocal, Duration: time.Minute}
	if err := loc.Validate(); err != ErrLocalNeedsPath {
		t.Errorf("want ErrLocalNeedsPath, got %v", err)
	}
	loc.Path = "/a.mp3"
	if err := loc.Validate(); err != nil {
		t.Errorf("valid local should pass, got %v", err)
	}

	sp := &Track{ID: "2", Source: shared.SourceSpotify, Duration: time.Minute}
	if err := sp.Validate(); err != ErrSpotifyNeedsURI {
		t.Errorf("want ErrSpotifyNeedsURI, got %v", err)
	}
	sp.SpotifyURI = "spotify:track:x"
	// Even if a caller forces Mixable=true, Validate must reset it.
	sp.Mixable = true
	sp.PCMAvailable = true
	if err := sp.Validate(); err != nil {
		t.Fatal(err)
	}
	if sp.Mixable || sp.PCMAvailable {
		t.Fatal("Validate must re-derive flags and block spotify mixable")
	}

	yt := &Track{ID: "3", Source: shared.SourceYouTube, Duration: time.Minute}
	if err := yt.Validate(); err != ErrYouTubeNeedsURI {
		t.Errorf("want ErrYouTubeNeedsURI, got %v", err)
	}
	yt.YouTubeURI = "https://youtu.be/x"
	// youtube ⇒ !mixable, same invariant as spotify.
	yt.Mixable = true
	yt.PCMAvailable = true
	if err := yt.Validate(); err != nil {
		t.Fatal(err)
	}
	if yt.Mixable || yt.PCMAvailable {
		t.Fatal("Validate must re-derive flags and block youtube mixable")
	}

	bad := &Track{ID: "", Source: shared.SourceLocal, Duration: time.Minute}
	if err := bad.Validate(); err != ErrEmptyID {
		t.Errorf("want ErrEmptyID, got %v", err)
	}
	badSrc := &Track{ID: "1", Source: "zz", Duration: time.Minute}
	if err := badSrc.Validate(); err != ErrBadSource {
		t.Errorf("want ErrBadSource, got %v", err)
	}
	badDur := &Track{ID: "1", Source: shared.SourceLocal, Duration: 0, Path: "/a"}
	if err := badDur.Validate(); err != ErrBadDuration {
		t.Errorf("want ErrBadDuration, got %v", err)
	}
}

func TestAnalyzed(t *testing.T) {
	loc, _ := NewTrack("1", shared.SourceLocal, "x", time.Minute)
	if loc.Analyzed() {
		t.Fatal("fresh track not analyzed")
	}
	loc.BPM = 128
	if !loc.Analyzed() {
		t.Fatal("local with BPM is analyzed")
	}
	sp, _ := NewTrack("2", shared.SourceSpotify, "x", time.Minute)
	sp.BPM = 128 // even if forced, non-PCM is never "analyzed"
	if sp.Analyzed() {
		t.Fatal("spotify can never be analyzed")
	}
}

/*
HasInstrumental e Separable têm UMA régua só: existe PCM deste lado, e o arquivo
já foi gerado?

O modelo separa por CONTEÚDO, então imagem estéreo não entra na conta — nem para
permitir, nem para recusar. É por isso que isto funciona em faixa MONO, e é a
razão de o cancelamento de centro ter sido aposentado na 0.2.32.
*/
func TestInstrumentalEHSeparable(t *testing.T) {
	loc, err := NewTrack("1", shared.SourceLocal, "x", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	loc.Path = "/a.mp3"
	if loc.HasInstrumental() {
		t.Fatal("faixa sem arquivo separado não tem instrumental")
	}
	if !loc.Separable() {
		t.Fatal("faixa local com arquivo pode ser separada")
	}
	loc.InstrumentalPath = "/a.instrumental.wav"
	if !loc.HasInstrumental() {
		t.Fatal("com o arquivo gerado, tem instrumental")
	}

	// Streaming não tem PCM deste lado: nada a separar, nada a servir.
	sp, err := NewTrack("2", shared.SourceSpotify, "y", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	sp.SpotifyURI = "spotify:track:z"
	sp.InstrumentalPath = "/forçado.wav" // mesmo forçado
	if sp.HasInstrumental() || sp.Separable() {
		t.Fatal("streaming nunca é separável nem serve instrumental")
	}
}
