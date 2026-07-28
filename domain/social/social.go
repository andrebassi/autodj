// Package social is the social-integrations bounded context (Story 8.5): it turns
// a "now playing" summary or a free-text note into a platform-shaped Post
// (WhatsApp / X / Facebook), enforcing per-platform character limits and
// truncation. It is pure — no clock, no persistence, no HTTP. Actually publishing
// a Post (and the credentials that needs) lives behind the outbound.SocialGateway
// port; the home build wires a stub. This is metadata, never audio, so it is
// independent of the two-engine (spotify ⇒ !mixable) invariant.
package social

import (
	"errors"
	"strings"
	"time"
)

// Platform identifies a social network target.
type Platform string

const (
	// PlatformWhatsApp posts to WhatsApp (status/broadcast).
	PlatformWhatsApp Platform = "whatsapp"
	// PlatformX posts to X (formerly Twitter) — the 280-char limit.
	PlatformX Platform = "x"
	// PlatformFacebook posts to a Facebook page.
	PlatformFacebook Platform = "facebook"
)

// Domain errors.
var (
	// ErrBadPlatform is returned for an unknown platform.
	ErrBadPlatform = errors.New("social: unknown platform")
	// ErrNoTrack is returned when composing a now-playing post with a blank title.
	ErrNoTrack = errors.New("social: track title is required")
	// ErrEmptyText is returned when composing a custom post with blank text.
	ErrEmptyText = errors.New("social: text is required")
)

// maxLen is the per-platform character (rune) limit. X is the tight one; the
// others are generous but still bounded so a runaway note can't be unbounded.
var maxLen = map[Platform]int{
	PlatformWhatsApp: 65536,
	PlatformX:        280,
	PlatformFacebook: 63206,
}

// label is the human brand name per platform.
var label = map[Platform]string{
	PlatformWhatsApp: "WhatsApp",
	PlatformX:        "X",
	PlatformFacebook: "Facebook",
}

// Valid reports whether p is a known platform.
func (p Platform) Valid() bool {
	_, ok := maxLen[p]
	return ok
}

// MaxLen is the rune limit for p (0 if unknown).
func (p Platform) MaxLen() int { return maxLen[p] }

// Label is the brand name for p (empty if unknown).
func (p Platform) Label() string { return label[p] }

// Platforms is the canonical platform list, in display order. The UI reads this
// instead of hardcoding a list.
func Platforms() []Platform {
	return []Platform{PlatformWhatsApp, PlatformX, PlatformFacebook}
}

// NowInfo is the minimal now-playing summary a post is built from. Source is
// informational (the two engines are visible elsewhere); the current build's
// text uses the title.
type NowInfo struct {
	Title  string
	Source string
}

// Post is a platform-shaped, ready-to-publish message. Immutable value.
type Post struct {
	Platform  Platform
	Text      string
	CreatedAt time.Time
}

// Receipt is the outcome of publishing a Post — the gateway's proof. ExternalID
// and URL identify the published item; PublishedAt is when it went out.
type Receipt struct {
	Platform    Platform
	ExternalID  string
	URL         string
	PublishedAt time.Time
}

// hashtag is the per-platform suffix appended to now-playing posts. Only X gets
// one (its culture); the others stay clean.
var hashtag = map[Platform]string{
	PlatformX: " #NaRádio",
}

// ComposeNowPlaying builds a now-playing announcement for p. The title is
// required; the platform must be valid. The text is truncated (by rune) to the
// platform limit with an ellipsis.
func ComposeNowPlaying(p Platform, now NowInfo, createdAt time.Time) (Post, error) {
	if !p.Valid() {
		return Post{}, ErrBadPlatform
	}
	title := strings.TrimSpace(now.Title)
	if title == "" {
		return Post{}, ErrNoTrack
	}
	text := "🎵 Agora na rádio: " + title + hashtag[p]
	return Post{Platform: p, Text: truncate(text, p.MaxLen()), CreatedAt: createdAt}, nil
}

// ComposeCustom builds a free-text post for p. The text is trimmed and required;
// the platform must be valid. Truncated by rune to the platform limit.
func ComposeCustom(p Platform, text string, createdAt time.Time) (Post, error) {
	if !p.Valid() {
		return Post{}, ErrBadPlatform
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return Post{}, ErrEmptyText
	}
	return Post{Platform: p, Text: truncate(text, p.MaxLen()), CreatedAt: createdAt}, nil
}

// truncate shortens s to at most max runes, replacing the tail with "…" when it
// overflows. max<=0 means no limit.
func truncate(s string, max int) string {
	if max <= 0 {
		return s
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max-1]) + "…"
}
