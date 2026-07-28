// Package tts implements the outbound TTSGateway: it synthesizes the AI DJ's
// spoken segues (Story 6.4) out-of-process. Two adapters — ElevenLabs (cloud
// HTTP, natural pt-BR) and Piper (offline exec, CPU) — return raw audio the app
// caches by hash. The synthesized audio is PCM, so the resulting source=tts track
// is mixable on the local playout bus: it is the *voice* that bridges a DRM
// Spotify track without the DRM audio ever crossing into the mix.
package tts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"autodj/domain/aidj"
)

// elevenBase is the ElevenLabs text-to-speech API root. Overridable for tests.
const elevenBase = "https://api.elevenlabs.io/v1"

// elevenModel is the low-latency multilingual model (pt-BR capable) used for
// live segues.
const elevenModel = "eleven_flash_v2_5"

// voiceIDs maps our fictional voice ids (anti-copyright — no real person/brand)
// to ElevenLabs voice ids. Unknown voices fall back to the default voice.
var voiceIDs = map[aidj.Voice]string{
	aidj.DefaultVoice: "aurora000000000000000",
	aidj.VoiceNimbus:  "nimbus000000000000000",
	aidj.VoiceFaber:   "faber0000000000000000",
}

// ElevenLabs implements outbound.TTSGateway against api.elevenlabs.io. The API
// key authenticates; base and HTTP are injectable for httptest.
type ElevenLabs struct {
	HTTP    *http.Client
	apiBase string
	apiKey  string
}

// NewElevenLabs builds a cloud TTS gateway with the given API key.
func NewElevenLabs(apiKey string) *ElevenLabs {
	return &ElevenLabs{
		HTTP:    &http.Client{Timeout: 30 * time.Second},
		apiBase: elevenBase,
		apiKey:  apiKey,
	}
}

func (e *ElevenLabs) httpClient() *http.Client {
	if e.HTTP != nil {
		return e.HTTP
	}
	return http.DefaultClient
}

func (e *ElevenLabs) base() string {
	if e.apiBase != "" {
		return e.apiBase
	}
	return elevenBase
}

// voiceID resolves our voice to an ElevenLabs voice id, defaulting when unknown.
func voiceID(v aidj.Voice) string {
	if id, ok := voiceIDs[v]; ok {
		return id
	}
	return voiceIDs[aidj.DefaultVoice]
}

// Synthesize POSTs the text to ElevenLabs and returns the audio stream. The
// caller stores and closes it. A non-2xx response is an error.
func (e *ElevenLabs) Synthesize(ctx context.Context, text string, voice aidj.Voice) (io.ReadCloser, error) {
	body, _ := json.Marshal(map[string]any{
		"text":     text,
		"model_id": elevenModel,
	})
	url := fmt.Sprintf("%s/text-to-speech/%s", e.base(), voiceID(voice))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("xi-api-key", e.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "audio/wav")

	resp, err := e.httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		_ = resp.Body.Close()
		return nil, fmt.Errorf("tts: elevenlabs %d: %s", resp.StatusCode, string(msg))
	}
	return resp.Body, nil
}
