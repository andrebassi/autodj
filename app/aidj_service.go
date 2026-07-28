package app

import (
	"context"
	"sync"
	"time"

	"autodj/domain/aidj"
	"autodj/domain/library"
	"autodj/domain/shared"
	"autodj/ports/inbound"
	"autodj/ports/outbound"
)

// AIDJService is the spoken auto-DJ (Story 6.4). It renders a segue line for a
// target track, synthesizes it through the TTS gateway (cache-by-hash in the
// MediaStore), and materializes it as a source=tts Track — PCM, mixable, so the
// *voice* rides the local playout bus. That voice is what bridges a DRM Spotify
// track: the Spotify SDK only fades its own volume while the voiceover plays
// locally; DRM audio never crosses into the mix (two-engine invariant intact).
//
// Config (enabled + voice) is held in memory — single operator, home use, like
// SpotifyService's pending flow.
type AIDJService struct {
	tracks outbound.TrackRepo
	media  outbound.MediaStore
	tts    outbound.TTSGateway // optional; nil → Voiceover returns aidj.ErrTTSUnavailable
	script aidj.Script

	mu  sync.Mutex
	cfg inbound.AIDJConfig
}

// NewAIDJService builds the service with the default segue script. tts is
// optional: without it, Voiceover returns aidj.ErrTTSUnavailable (the config
// still reads/writes so the panel works). Spoken segues start disabled.
func NewAIDJService(tracks outbound.TrackRepo, media outbound.MediaStore, tts outbound.TTSGateway) *AIDJService {
	return &AIDJService{
		tracks: tracks,
		media:  media,
		tts:    tts,
		script: aidj.DefaultScript,
		cfg:    inbound.AIDJConfig{Enabled: false, Voice: aidj.DefaultVoice},
	}
}

// Config reports the current enabled flag and voice.
func (s *AIDJService) Config(_ context.Context) inbound.AIDJConfig {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg
}

// SetConfig validates the voice (aidj.ErrBadVoice when unknown) and stores the
// config, returning the stored value. An empty voice normalizes to DefaultVoice.
func (s *AIDJService) SetConfig(_ context.Context, cfg inbound.AIDJConfig) (inbound.AIDJConfig, error) {
	if !aidj.ValidVoice(cfg.Voice) {
		return inbound.AIDJConfig{}, aidj.ErrBadVoice
	}
	if cfg.Voice == "" {
		cfg.Voice = aidj.DefaultVoice
	}
	s.mu.Lock()
	s.cfg = cfg
	s.mu.Unlock()
	return cfg, nil
}

// Voiceover renders and synthesizes the spoken segue for forTrackID, returning a
// source=tts mixable Track to enqueue ahead of the target. It short-circuits when
// spoken segues are off (aidj.ErrDisabled) or no gateway is wired
// (aidj.ErrTTSUnavailable). Synthesis is cached by text+voice hash: a repeated
// line reuses the stored track and audio instead of paying the gateway again.
func (s *AIDJService) Voiceover(ctx context.Context, forTrackID string) (library.Track, error) {
	s.mu.Lock()
	cfg := s.cfg
	s.mu.Unlock()

	if !cfg.Enabled {
		return library.Track{}, aidj.ErrDisabled
	}
	if s.tts == nil {
		return library.Track{}, aidj.ErrTTSUnavailable
	}

	target, err := s.tracks.Get(ctx, forTrackID)
	if err != nil {
		return library.Track{}, library.ErrTrackNotFound
	}

	text := s.script.Render(aidj.TrackMeta{Title: target.Title, Artist: target.Artist})
	vo, err := aidj.NewVoiceover(text, cfg.Voice)
	if err != nil {
		return library.Track{}, err
	}
	ttsID := "tts-" + vo.Hash

	// Cache hit: identical line+voice already synthesized → reuse the stored track.
	if cached, err := s.tracks.Get(ctx, ttsID); err == nil {
		return *cached, nil
	}

	rc, err := s.tts.Synthesize(ctx, text, cfg.Voice)
	if err != nil {
		return library.Track{}, err
	}
	defer rc.Close()
	path, err := s.media.Save(ctx, vo.Hash+".wav", rc)
	if err != nil {
		return library.Track{}, err
	}

	dur := time.Duration(vo.DurationMs) * time.Millisecond
	t, err := library.NewTrack(ttsID, shared.SourceTTS, "Locução: "+target.Title, dur)
	if err != nil {
		return library.Track{}, err
	}
	t.Artist = "AI DJ"
	t.Category = string(library.CategoryPromo)
	t.Path = path
	if err := t.Validate(); err != nil {
		return library.Track{}, err
	}
	if err := s.tracks.Upsert(ctx, t); err != nil {
		return library.Track{}, err
	}
	return *t, nil
}
