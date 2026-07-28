package tts

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"autodj/domain/aidj"
)

func TestVoiceID(t *testing.T) {
	if voiceID(aidj.VoiceNimbus) != voiceIDs[aidj.VoiceNimbus] {
		t.Error("known voice must map to its id")
	}
	if voiceID("unknown-voice") != voiceIDs[aidj.DefaultVoice] {
		t.Error("unknown voice must fall back to default id")
	}
}

func TestElevenLabsSynthesize(t *testing.T) {
	var gotKey, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("xi-api-key")
		gotPath = r.URL.Path
		_, _ = w.Write([]byte("WAVDATA"))
	}))
	defer srv.Close()

	e := NewElevenLabs("secret-key")
	e.apiBase = srv.URL
	rc, err := e.Synthesize(context.Background(), "olá ouvinte", aidj.VoiceNimbus)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	defer rc.Close()
	b, _ := io.ReadAll(rc)
	if string(b) != "WAVDATA" {
		t.Errorf("audio = %q", b)
	}
	if gotKey != "secret-key" {
		t.Errorf("xi-api-key = %q", gotKey)
	}
	if !strings.Contains(gotPath, voiceIDs[aidj.VoiceNimbus]) {
		t.Errorf("path %q missing voice id", gotPath)
	}
}

func TestElevenLabsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("bad key"))
	}))
	defer srv.Close()

	e := NewElevenLabs("x")
	e.apiBase = srv.URL
	if _, err := e.Synthesize(context.Background(), "t", aidj.DefaultVoice); err == nil {
		t.Fatal("expected error on 401")
	}
}

func TestElevenLabsDefaults(t *testing.T) {
	e := &ElevenLabs{}
	if e.httpClient() != http.DefaultClient {
		t.Error("nil HTTP must default")
	}
	if e.base() != elevenBase {
		t.Error("empty apiBase must default")
	}
}

func TestPiperAvailable(t *testing.T) {
	// no model configured → unavailable regardless of binary
	if (&Piper{Bin: "sh"}).Available() {
		t.Error("no model → unavailable")
	}
	// model set but bogus binary → unavailable
	p := NewPiper(map[aidj.Voice]string{aidj.DefaultVoice: "/tmp/m.onnx"})
	p.Bin = "definitely-not-a-real-binary-xyz"
	if p.Available() {
		t.Error("missing binary → unavailable")
	}
}

func TestPiperModelFallback(t *testing.T) {
	p := NewPiper(map[aidj.Voice]string{aidj.DefaultVoice: "def.onnx", aidj.VoiceNimbus: "nim.onnx"})
	if p.model(aidj.VoiceNimbus) != "nim.onnx" {
		t.Error("mapped voice must use its model")
	}
	if p.model(aidj.VoiceFaber) != "def.onnx" {
		t.Error("unmapped voice must fall back to default model")
	}
	if p.bin() != "piper" {
		t.Errorf("bin default = %q", p.bin())
	}
}

func TestPiperSynthesizeMissingBinary(t *testing.T) {
	p := NewPiper(map[aidj.Voice]string{aidj.DefaultVoice: "m.onnx"})
	p.Bin = "definitely-not-a-real-binary-xyz"
	if _, err := p.Synthesize(context.Background(), "t", aidj.DefaultVoice); err == nil {
		t.Fatal("expected error for missing piper binary")
	}
}

func TestPiperSynthesizeNoModel(t *testing.T) {
	p := &Piper{Bin: "sh", Models: map[aidj.Voice]string{}}
	// sh exists on PATH, but no model configured → error before exec
	if _, err := p.Synthesize(context.Background(), "t", aidj.DefaultVoice); err == nil {
		t.Fatal("expected error for missing model")
	}
}

func TestResolve(t *testing.T) {
	if resolve("") != "" {
		t.Error("empty bin resolves empty")
	}
	if resolve("definitely-not-a-real-binary-xyz") != "" {
		t.Error("absent bin resolves empty")
	}
}
