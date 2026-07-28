// Package aidj models the AI DJ: spoken voiceovers (source=tts) and the segue
// plan that bridges one track into the next. It is pure — no I/O, no external
// deps beyond domain/shared — so the two-engine rules live in one testable place.
//
// A voiceover is synthesized speech: it has its own PCM, so per the capability
// matrix it is mixable and enters the local playout bus like any local file.
// That is what lets the AI DJ stitch a DRM Spotify track: the *voice* plays on
// the local bus while the Spotify SDK only fades its own volume — the Spotify
// audio is never summed or relayed. A track with no PCM (Spotify) can therefore
// only be reached by fade_tts or hard_cut, never a PCM crossfade.
package aidj

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"strings"

	"autodj/domain/shared"
)

// Errors returned when building or serving a voiceover.
var (
	// ErrEmptyText rejects a voiceover with no spoken text.
	ErrEmptyText = errors.New("aidj: empty voiceover text")
	// ErrBadVoice rejects a SetConfig with an unknown voice.
	ErrBadVoice = errors.New("aidj: unknown voice")
	// ErrTTSUnavailable means no TTS gateway is wired (synthesis impossible).
	ErrTTSUnavailable = errors.New("aidj: tts gateway unavailable")
	// ErrDisabled means spoken segues are turned off in the config.
	ErrDisabled = errors.New("aidj: spoken segues disabled")
)

// Voice identifies a TTS voice (fictional ids — anti-copyright, no real
// person/brand). The empty voice normalizes to DefaultVoice.
type Voice string

const (
	// DefaultVoice is the pt-BR narration voice used when none is chosen.
	DefaultVoice Voice = "aurora-ptbr"
	// VoiceFaber is the offline (Piper) fallback voice.
	VoiceFaber Voice = "faber-ptbr"
	// VoiceNimbus is a second cloud voice option.
	VoiceNimbus Voice = "nimbus-ptbr"
)

// Voices lists the selectable voices for the UI. Order is stable.
func Voices() []Voice { return []Voice{DefaultVoice, VoiceNimbus, VoiceFaber} }

// ValidVoice reports whether v is a known voice (empty is valid — it defaults).
func ValidVoice(v Voice) bool {
	if v == "" {
		return true
	}
	for _, k := range Voices() {
		if k == v {
			return true
		}
	}
	return false
}

// TrackMeta is the minimal metadata a Script needs to render a spoken line.
type TrackMeta struct {
	Title  string
	Artist string
}

// Script is a spoken-line template. {title} and {artist} are filled from
// TrackMeta; empty fields fall back to neutral phrasing so the line always reads.
type Script struct {
	Template string
}

// DefaultScript is the stock segue line ("A seguir, ...").
var DefaultScript = Script{Template: "A seguir, {title} com {artist}, aqui na sua rádio."}

// Render fills the template from m. Empty title/artist use neutral fallbacks.
func (s Script) Render(m TrackMeta) string {
	tmpl := s.Template
	if strings.TrimSpace(tmpl) == "" {
		tmpl = DefaultScript.Template
	}
	title := strings.TrimSpace(m.Title)
	if title == "" {
		title = "a próxima faixa"
	}
	artist := strings.TrimSpace(m.Artist)
	if artist == "" {
		artist = "um artista da casa"
	}
	out := strings.ReplaceAll(tmpl, "{title}", title)
	out = strings.ReplaceAll(out, "{artist}", artist)
	return out
}

// wordsPerMinute approximates pt-BR narration pace, used to estimate a
// voiceover's duration from its text without synthesizing audio (keeps the
// domain pure and deterministic).
const wordsPerMinute = 150

// minDurationMs floors a voiceover duration (a one-word line still takes time).
const minDurationMs = 800

// EstimateDurationMs estimates spoken duration (ms) from text via word count.
// Deterministic: same text → same estimate. Never below minDurationMs.
func EstimateDurationMs(text string) int64 {
	words := int64(len(strings.Fields(text)))
	if words == 0 {
		return minDurationMs
	}
	ms := words * 60000 / wordsPerMinute
	if ms < minDurationMs {
		return minDurationMs
	}
	return ms
}

// Hash is a deterministic cache key over text+voice: the same line in the same
// voice hashes identically, so the app reuses a cached synthesis instead of
// paying the TTS gateway again.
func Hash(text string, voice Voice) string {
	if voice == "" {
		voice = DefaultVoice
	}
	sum := sha1.Sum([]byte(text + "\x00" + string(voice)))
	return hex.EncodeToString(sum[:])
}

// Voiceover is a synthesized spoken segment. It is source=tts: its own PCM makes
// it mixable on the local bus. AudioRef is empty until the gateway synthesizes it.
type Voiceover struct {
	ID         string
	Text       string
	Voice      Voice
	Hash       string
	DurationMs int64
	AudioRef   string
}

// NewVoiceover builds a voiceover from text+voice, computing its cache hash and
// estimated duration. ID equals Hash so identical lines dedupe in the catalog.
func NewVoiceover(text string, voice Voice) (Voiceover, error) {
	if strings.TrimSpace(text) == "" {
		return Voiceover{}, ErrEmptyText
	}
	if voice == "" {
		voice = DefaultVoice
	}
	h := Hash(text, voice)
	return Voiceover{
		ID:         h,
		Text:       text,
		Voice:      voice,
		Hash:       h,
		DurationMs: EstimateDurationMs(text),
	}, nil
}

// TransitionPlan is how the auto-DJ bridges into the next track: the transition
// kind plus the voiceover to speak (empty when there's no spoken bridge).
type TransitionPlan struct {
	Kind        shared.Transition
	VoiceoverID string
}

// PlanTransition derives the bridge into a track of source kind next. The
// invariant: a non-PCM (Spotify) next track can NEVER be crossfaded.
//   - With a voiceover → fade_tts (duck current, speak, bring up next). This is
//     the only way to stitch a DRM Spotify track, and works for PCM too.
//   - No voiceover + Spotify next → hard_cut (no PCM to overlap).
//   - No voiceover + PCM next → crossfade (mix settings from Épico 3 decide the
//     shape; here we just say a PCM overlap is permitted).
func PlanTransition(next shared.SourceKind, voiceoverID string) TransitionPlan {
	switch {
	case voiceoverID != "":
		return TransitionPlan{Kind: shared.TransitionFadeTTS, VoiceoverID: voiceoverID}
	case !shared.Capabilities(next).Crossfade:
		return TransitionPlan{Kind: shared.TransitionHardCut}
	default:
		return TransitionPlan{Kind: shared.TransitionCrossfade}
	}
}
