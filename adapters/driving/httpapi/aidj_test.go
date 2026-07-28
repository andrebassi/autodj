package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"autodj/domain/aidj"
	"autodj/domain/library"
	"autodj/domain/shared"
	"autodj/ports/inbound"
)

// fakeAIDJ is a configurable AIDJService stub.
type fakeAIDJ struct {
	cfg      inbound.AIDJConfig
	setErr   error
	voErr    error
	vo       library.Track
	lastFor  string // last Voiceover arg
	setCalls int
}

func (f *fakeAIDJ) Config(context.Context) inbound.AIDJConfig { return f.cfg }
func (f *fakeAIDJ) SetConfig(_ context.Context, cfg inbound.AIDJConfig) (inbound.AIDJConfig, error) {
	f.setCalls++
	if f.setErr != nil {
		return inbound.AIDJConfig{}, f.setErr
	}
	f.cfg = cfg
	return cfg, nil
}
func (f *fakeAIDJ) Voiceover(_ context.Context, forTrackID string) (library.Track, error) {
	f.lastFor = forTrackID
	if f.voErr != nil {
		return library.Track{}, f.voErr
	}
	return f.vo, nil
}

func newAIDJTrack(t *testing.T) library.Track {
	t.Helper()
	tr, err := library.NewTrack("tts-abc", shared.SourceTTS, "Locução: Chuva de Neon", 4*time.Second)
	if err != nil {
		t.Fatalf("NewTrack: %v", err)
	}
	tr.Artist = "AI DJ"
	tr.Category = string(library.CategoryPromo)
	tr.Path = "/media/abc.wav"
	return *tr
}

func TestGetAIDJConfig(t *testing.T) {
	svc := &fakeAIDJ{cfg: inbound.AIDJConfig{Enabled: true, Voice: aidj.VoiceNimbus}}
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil, WithAIDJ(svc))
	rec := do(t, h, http.MethodGet, "/api/v1/aidj/config", "")
	if rec.Code != 200 {
		t.Fatalf("code=%d", rec.Code)
	}
	var got inbound.AIDJConfig
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.Enabled || got.Voice != aidj.VoiceNimbus {
		t.Fatalf("cfg=%+v", got)
	}
}

func TestPutAIDJConfig(t *testing.T) {
	svc := &fakeAIDJ{}
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil, WithAIDJ(svc))
	rec := do(t, h, http.MethodPut, "/api/v1/aidj/config", `{"enabled":true,"voice":"nimbus-ptbr"}`)
	if rec.Code != 200 {
		t.Fatalf("code=%d", rec.Code)
	}
	if svc.setCalls != 1 || !svc.cfg.Enabled || svc.cfg.Voice != aidj.VoiceNimbus {
		t.Fatalf("stored=%+v calls=%d", svc.cfg, svc.setCalls)
	}
}

func TestPutAIDJConfigBadBody(t *testing.T) {
	svc := &fakeAIDJ{}
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil, WithAIDJ(svc))
	rec := do(t, h, http.MethodPut, "/api/v1/aidj/config", `{`)
	if rec.Code != 400 {
		t.Fatalf("malformed body should be 400, got %d", rec.Code)
	}
	if svc.setCalls != 0 {
		t.Fatal("bad body must not reach service")
	}
}

func TestPutAIDJConfigBadVoice(t *testing.T) {
	svc := &fakeAIDJ{setErr: aidj.ErrBadVoice}
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil, WithAIDJ(svc))
	rec := do(t, h, http.MethodPut, "/api/v1/aidj/config", `{"enabled":true,"voice":"bogus"}`)
	if rec.Code != 422 {
		t.Fatalf("unknown voice should be 422, got %d", rec.Code)
	}
}

func TestPostAIDJVoiceover(t *testing.T) {
	svc := &fakeAIDJ{vo: newAIDJTrack(t)}
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil, WithAIDJ(svc))
	rec := do(t, h, http.MethodPost, "/api/v1/aidj/voiceover", `{"trackId":"next1"}`)
	if rec.Code != 201 {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	if svc.lastFor != "next1" {
		t.Fatalf("Voiceover arg=%q", svc.lastFor)
	}
	var dto trackDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &dto); err != nil {
		t.Fatal(err)
	}
	if dto.Source != string(shared.SourceTTS) {
		t.Fatalf("source=%q, want tts", dto.Source)
	}
}

func TestPostAIDJVoiceoverMissingTrackID(t *testing.T) {
	svc := &fakeAIDJ{}
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil, WithAIDJ(svc))
	rec := do(t, h, http.MethodPost, "/api/v1/aidj/voiceover", `{}`)
	if rec.Code != 400 {
		t.Fatalf("missing trackId should be 400, got %d", rec.Code)
	}
	if svc.lastFor != "" {
		t.Fatal("must not reach service without trackId")
	}
}

func TestPostAIDJVoiceoverDisabled(t *testing.T) {
	svc := &fakeAIDJ{voErr: aidj.ErrDisabled}
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil, WithAIDJ(svc))
	rec := do(t, h, http.MethodPost, "/api/v1/aidj/voiceover", `{"trackId":"x"}`)
	if rec.Code != 409 {
		t.Fatalf("disabled should be 409, got %d", rec.Code)
	}
}

func TestPostAIDJVoiceoverNoGateway(t *testing.T) {
	svc := &fakeAIDJ{voErr: aidj.ErrTTSUnavailable}
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil, WithAIDJ(svc))
	rec := do(t, h, http.MethodPost, "/api/v1/aidj/voiceover", `{"trackId":"x"}`)
	if rec.Code != 503 {
		t.Fatalf("no gateway should be 503, got %d", rec.Code)
	}
}

func TestPostAIDJVoiceoverTrackNotFound(t *testing.T) {
	svc := &fakeAIDJ{voErr: library.ErrTrackNotFound}
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil, WithAIDJ(svc))
	rec := do(t, h, http.MethodPost, "/api/v1/aidj/voiceover", `{"trackId":"missing"}`)
	if rec.Code != 404 {
		t.Fatalf("unknown track should be 404, got %d", rec.Code)
	}
}

func TestAIDJRoutesAbsentWithoutOption(t *testing.T) {
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil)
	if rec := do(t, h, http.MethodGet, "/api/v1/aidj/config", ""); rec.Code != 404 {
		t.Fatalf("GET config should 404 without WithAIDJ, got %d", rec.Code)
	}
	if rec := do(t, h, http.MethodPost, "/api/v1/aidj/voiceover", `{"trackId":"x"}`); rec.Code != 404 {
		t.Fatalf("POST voiceover should 404 without WithAIDJ, got %d", rec.Code)
	}
}
