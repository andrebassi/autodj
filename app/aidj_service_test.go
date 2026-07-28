package app

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"autodj/domain/aidj"
	"autodj/domain/library"
	"autodj/domain/shared"
	"autodj/adapters/driven/memory"
	"autodj/ports/inbound"
)

// fakeTTS is an in-memory TTSGateway: Synthesize returns fixed audio bytes and
// counts calls (to assert the cache-by-hash avoids a second synthesis). synthErr
// forces a gateway failure.
type fakeTTS struct {
	audio    []byte
	synthErr error
	calls    int
}

func (f *fakeTTS) Synthesize(_ context.Context, _ string, _ aidj.Voice) (io.ReadCloser, error) {
	f.calls++
	if f.synthErr != nil {
		return nil, f.synthErr
	}
	return io.NopCloser(bytes.NewReader(f.audio)), nil
}

func seedLocalTrack(t *testing.T, repo interface {
	Upsert(context.Context, *library.Track) error
}, id, title, artist string) {
	t.Helper()
	tr, err := library.NewTrack(id, shared.SourceLocal, title, 3*time.Minute)
	if err != nil {
		t.Fatalf("NewTrack: %v", err)
	}
	tr.Artist = artist
	tr.Path = "/media/" + id + ".mp3"
	if err := tr.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if err := repo.Upsert(context.Background(), tr); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
}

func TestAIDJConfigDefault(t *testing.T) {
	svc := NewAIDJService(memory.New(), newFakeMedia(), &fakeTTS{})
	cfg := svc.Config(context.Background())
	if cfg.Enabled {
		t.Error("spoken segues should start disabled")
	}
	if cfg.Voice != aidj.DefaultVoice {
		t.Errorf("Voice = %q, want default", cfg.Voice)
	}
}

func TestAIDJSetConfig(t *testing.T) {
	ctx := context.Background()
	svc := NewAIDJService(memory.New(), newFakeMedia(), &fakeTTS{})

	t.Run("bad voice rejected", func(t *testing.T) {
		if _, err := svc.SetConfig(ctx, inbound.AIDJConfig{Enabled: true, Voice: "bogus"}); err != aidj.ErrBadVoice {
			t.Fatalf("err = %v, want ErrBadVoice", err)
		}
	})

	t.Run("empty voice normalizes to default", func(t *testing.T) {
		got, err := svc.SetConfig(ctx, inbound.AIDJConfig{Enabled: true, Voice: ""})
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if got.Voice != aidj.DefaultVoice {
			t.Errorf("Voice = %q, want default", got.Voice)
		}
	})

	t.Run("valid config stored and read back", func(t *testing.T) {
		got, err := svc.SetConfig(ctx, inbound.AIDJConfig{Enabled: true, Voice: aidj.VoiceNimbus})
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !got.Enabled || got.Voice != aidj.VoiceNimbus {
			t.Fatalf("stored = %+v", got)
		}
		if svc.Config(ctx) != got {
			t.Error("Config() must reflect SetConfig")
		}
	})
}

func TestAIDJVoiceoverDisabled(t *testing.T) {
	svc := NewAIDJService(memory.New(), newFakeMedia(), &fakeTTS{})
	if _, err := svc.Voiceover(context.Background(), "t1"); err != aidj.ErrDisabled {
		t.Fatalf("err = %v, want ErrDisabled", err)
	}
}

func TestAIDJVoiceoverNoGateway(t *testing.T) {
	ctx := context.Background()
	svc := NewAIDJService(memory.New(), newFakeMedia(), nil)
	if _, err := svc.SetConfig(ctx, inbound.AIDJConfig{Enabled: true, Voice: aidj.DefaultVoice}); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	if _, err := svc.Voiceover(ctx, "t1"); err != aidj.ErrTTSUnavailable {
		t.Fatalf("err = %v, want ErrTTSUnavailable", err)
	}
}

func TestAIDJVoiceoverTrackNotFound(t *testing.T) {
	ctx := context.Background()
	svc := NewAIDJService(memory.New(), newFakeMedia(), &fakeTTS{audio: []byte("pcm")})
	_, _ = svc.SetConfig(ctx, inbound.AIDJConfig{Enabled: true, Voice: aidj.DefaultVoice})
	if _, err := svc.Voiceover(ctx, "missing"); err != library.ErrTrackNotFound {
		t.Fatalf("err = %v, want ErrTrackNotFound", err)
	}
}

func TestAIDJVoiceoverHappyPath(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	media := newFakeMedia()
	tts := &fakeTTS{audio: []byte("synth-pcm-bytes")}
	svc := NewAIDJService(repo, media, tts)
	_, _ = svc.SetConfig(ctx, inbound.AIDJConfig{Enabled: true, Voice: aidj.VoiceNimbus})
	seedLocalTrack(t, repo, "next1", "Chuva de Neon", "Os Ventania")

	vo, err := svc.Voiceover(ctx, "next1")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	// source=tts → PCM, mixable (rides local bus; bridges DRM without crossing it).
	if vo.Source != shared.SourceTTS {
		t.Errorf("Source = %q, want tts", vo.Source)
	}
	if !vo.Mixable || !vo.PCMAvailable {
		t.Errorf("tts track must be PCM+mixable, got mixable=%v pcm=%v", vo.Mixable, vo.PCMAvailable)
	}
	if vo.Category != string(library.CategoryPromo) {
		t.Errorf("Category = %q, want promo", vo.Category)
	}
	if vo.Path == "" {
		t.Error("Path must point at the synthesized audio")
	}
	if len(media.saved) != 1 {
		t.Errorf("expected 1 saved audio file, got %d", len(media.saved))
	}
	// mentions the next track's title/artist in the spoken line's derived id
	if vo.ID == "" {
		t.Error("voiceover track must have an id")
	}
	// persisted so it can be enqueued and re-read
	if got, err := repo.Get(ctx, vo.ID); err != nil || got.Source != shared.SourceTTS {
		t.Errorf("voiceover not persisted: %v", err)
	}
}

func TestAIDJVoiceoverCacheHit(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	media := newFakeMedia()
	tts := &fakeTTS{audio: []byte("pcm")}
	svc := NewAIDJService(repo, media, tts)
	_, _ = svc.SetConfig(ctx, inbound.AIDJConfig{Enabled: true, Voice: aidj.DefaultVoice})
	seedLocalTrack(t, repo, "next1", "Aurora Fria", "Duo Espelho")

	first, err := svc.Voiceover(ctx, "next1")
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	second, err := svc.Voiceover(ctx, "next1")
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if first.ID != second.ID {
		t.Errorf("cache should return same track: %s vs %s", first.ID, second.ID)
	}
	if tts.calls != 1 {
		t.Errorf("gateway called %d times, want 1 (cache-by-hash)", tts.calls)
	}
	if len(media.saved) != 1 {
		t.Errorf("expected 1 saved file (no re-save on cache hit), got %d", len(media.saved))
	}
}

func TestAIDJVoiceoverSynthError(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	tts := &fakeTTS{synthErr: errors.New("elevenlabs 500")}
	svc := NewAIDJService(repo, newFakeMedia(), tts)
	_, _ = svc.SetConfig(ctx, inbound.AIDJConfig{Enabled: true, Voice: aidj.DefaultVoice})
	seedLocalTrack(t, repo, "next1", "T", "A")

	if _, err := svc.Voiceover(ctx, "next1"); err == nil {
		t.Fatal("expected synth error to propagate")
	}
}

func TestAIDJVoiceoverSaveError(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	media := &fakeMedia{saved: map[string][]byte{}, saveErr: errors.New("disk full")}
	tts := &fakeTTS{audio: []byte("pcm")}
	svc := NewAIDJService(repo, media, tts)
	_, _ = svc.SetConfig(ctx, inbound.AIDJConfig{Enabled: true, Voice: aidj.DefaultVoice})
	seedLocalTrack(t, repo, "next1", "T", "A")

	if _, err := svc.Voiceover(ctx, "next1"); err == nil {
		t.Fatal("expected media save error to propagate")
	}
}
