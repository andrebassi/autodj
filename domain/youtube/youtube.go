// Package youtube models the OAuth session that links a home operator to their
// Google/YouTube account. It is pure domain: no network, no crypto, no
// persistence — the OAuth code exchange (auth-code flow WITH client secret, not
// PKCE) happens over the outbound port. The two-engine split lives elsewhere (a
// YouTube Track is always non-mixable via the source matrix); this package
// tracks the account lifecycle so the panel can gate SEARCH behind a Google
// login (a quota shield: search costs ~100 units a call), plus the pure parsing
// of pasted video links — the paste-import path needs no account at all.
package youtube

import (
	"errors"
	"net/url"
	"strings"
	"time"
)

// refreshWindow is the slack before ExpiresAt at which a token is considered
// stale enough to refresh proactively, so AccessToken never hands out a token
// about to die mid-request.
const refreshWindow = 60 * time.Second

// pendingTTL bounds how long a started auth flow stays valid: the operator must
// return from the Google consent screen within this window or restart.
const pendingTTL = 10 * time.Minute

// Domain errors surfaced by the app service and mapped to HTTP status upstream.
var (
	// ErrNoAccount means no YouTube account is linked (the repo is empty).
	ErrNoAccount = errors.New("youtube: no account connected")
	// ErrStateMismatch means the callback state did not match the pending flow
	// (CSRF guard) — the callback is rejected.
	ErrStateMismatch = errors.New("youtube: oauth state mismatch")
	// ErrAuthExpired means the pending auth flow outlived its TTL before the
	// callback arrived.
	ErrAuthExpired = errors.New("youtube: pending auth expired")
	// ErrCatalogUnavailable means the Data API catalog gateway (search/import) is
	// not wired — OAuth works but no catalog client is configured.
	ErrCatalogUnavailable = errors.New("youtube: catalog gateway unavailable")
	// ErrQuotaExceeded means the YouTube Data API v3 daily quota is spent (upstream
	// 429, or 403 with a quota/rate reason). Search costs ~100 units and the free
	// tier is ~10k/day (~100 searches), so heavy use exhausts it until the Pacific
	// midnight reset. Mapped to HTTP 429 with a clear "try again tomorrow" message —
	// distinct from ErrCatalogUnavailable (gateway not wired at all).
	ErrQuotaExceeded = errors.New("youtube: data api daily quota exceeded")
	// ErrOAuthUnavailable means the Google OAuth client (id + secret) is not
	// configured, so connecting an account is impossible on this server. The
	// catalog (API key) may still work: paste-import and search do not need a
	// linked account.
	ErrOAuthUnavailable = errors.New("youtube: oauth client not configured")
	// ErrNoLinks means the pasted text carried no recognizable video link/id.
	ErrNoLinks = errors.New("youtube: no video links found")
)

// MaxImportLinks caps one paste-import batch. The Data API accepts at most 50
// ids per videos.list call and the service sends a single call, so anything past
// this is dropped instead of silently truncated mid-flight.
const MaxImportLinks = 50

// ParseLinks extracts YouTube video ids from free-form pasted text. The operator
// separates entries with ";" (the documented format) but real pastes also arrive
// newline- or comma-separated, so every one of those is treated as a separator.
//
// Each entry may be a watch URL, a youtu.be/shorts/embed short form, or a bare
// 11-char id. Playlist-only links (no "v=") are ignored: this path imports
// videos, and a whole playlist has its own import route.
//
// Duplicates collapse (same video pasted twice = one import) preserving the
// order of first appearance. The result is capped at MaxImportLinks.
func ParseLinks(raw string) []string {
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ';' || r == ',' || r == '\n' || r == '\r' || r == '\t' || r == ' '
	})
	out := make([]string, 0, len(fields))
	seen := make(map[string]struct{}, len(fields))
	for _, f := range fields {
		id := VideoID(f)
		if id == "" {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
		if len(out) == MaxImportLinks {
			break
		}
	}
	return out
}

// VideoID reduces one pasted entry to a bare video id, or "" when it carries
// none. Accepted shapes: watch?v=<id> (query order irrelevant), youtu.be/<id>,
// /shorts/<id>, /embed/<id>, /live/<id> and a bare <id>. A playlist link with no
// video component yields "" — playlists are imported through their own route.
func VideoID(entry string) string {
	s := strings.TrimSpace(entry)
	if s == "" {
		return ""
	}
	if !strings.Contains(s, "/") && !strings.Contains(s, "?") && !strings.Contains(s, "&") {
		return validID(s) // already bare
	}
	// url.Parse needs a scheme to split host from path; pastes often omit it.
	if !strings.Contains(s, "://") {
		s = "https://" + s
	}
	u, err := url.Parse(s)
	if err != nil {
		return ""
	}
	if !youtubeHost(u.Host) {
		return ""
	}
	if v := u.Query().Get("v"); v != "" {
		return validID(v)
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	last := parts[len(parts)-1]
	// youtu.be/<id> puts the id straight in the path; the long forms prefix it
	// with a segment. Either way the id is the last segment.
	if len(parts) == 1 && strings.EqualFold(u.Host, "youtu.be") {
		return validID(last)
	}
	switch {
	case len(parts) >= 2 && (parts[len(parts)-2] == "shorts" ||
		parts[len(parts)-2] == "embed" ||
		parts[len(parts)-2] == "live" ||
		parts[len(parts)-2] == "v"):
		return validID(last)
	}
	return ""
}

// youtubeHost reports whether a host belongs to YouTube. Anything else is
// rejected so a stray link in the paste never becomes a bogus id.
func youtubeHost(host string) bool {
	h := strings.ToLower(host)
	if i := strings.IndexByte(h, ':'); i >= 0 {
		h = h[:i] // strip port
	}
	h = strings.TrimPrefix(h, "www.")
	h = strings.TrimPrefix(h, "m.")
	return h == "youtube.com" || h == "youtu.be" || h == "music.youtube.com" ||
		h == "youtube-nocookie.com"
}

// validID accepts the 11-char id alphabet YouTube uses; anything else (a word,
// a truncated paste) is rejected rather than sent to the Data API as garbage.
func validID(s string) string {
	if len(s) != 11 {
		return ""
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
		default:
			return ""
		}
	}
	return s
}

// Account is the persisted OAuth session for the linked Google/YouTube user.
// Tokens are opaque strings from Google; ExpiresAt is when the access token dies.
// Scopes records what was granted. ChannelTitle is the linked channel's display
// name (for the panel). Premium is best-effort: YouTube exposes no clean
// membership flag and playback never depends on it, so this is kept only for chip
// parity with Spotify and is likely always false.
type Account struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
	Scopes       []string
	ChannelTitle string
	Premium      bool
}

// Connected reports whether an account is actually linked (has an access token).
// The zero Account is "not connected".
func (a Account) Connected() bool { return a.AccessToken != "" }

// IsPremium reports the best-effort membership flag. Playout NEVER gates on this
// (the IFrame Player plays any embeddable video for free); it exists only for
// chip parity with the Spotify panel.
func (a Account) IsPremium() bool { return a.Premium }

// Expired reports whether the access token is dead at now (inclusive of the
// exact instant). A zero ExpiresAt is treated as expired (no valid token yet).
func (a Account) Expired(now time.Time) bool {
	if a.ExpiresAt.IsZero() {
		return true
	}
	return !now.Before(a.ExpiresAt)
}

// NeedsRefresh reports whether the token is expired or within the refresh window
// of expiring, so AccessToken refreshes proactively.
func (a Account) NeedsRefresh(now time.Time) bool {
	if a.ExpiresAt.IsZero() {
		return true
	}
	return !now.Before(a.ExpiresAt.Add(-refreshWindow))
}

// HasScope reports whether scope s was granted to this account.
func (a Account) HasScope(s string) bool {
	for _, got := range a.Scopes {
		if got == s {
			return true
		}
	}
	return false
}

// Catalog search kinds. A search targets videos or playlists; import expands a
// playlist into its videos. Kept here so both ports agree on the vocabulary.
const (
	KindVideo    = "video"
	KindPlaylist = "playlist"
)

// Search backends. Which gateway answers a search — the operator chooses in the
// panel, both are always wired when the tab is up:
//
//   - BackendFree asks YouTube the way its own site does (InnerTube). No login,
//     no API key, no quota. Durations come from the page, so a non-embeddable
//     video can slip through when the Data API cannot enrich the page.
//   - BackendAPI spends Data API quota (search.list ≈ 100 of ~10k units/day) and
//     is gated behind the Google login precisely because that budget is shared.
//
// Kept here so port, adapter and HTTP layer agree on the vocabulary.
const (
	BackendFree = "free"
	BackendAPI  = "api"
)

// NormalizeBackend maps an operator-supplied backend name to a known value.
// Anything unrecognized — including "" from a client that predates the switch —
// becomes BackendFree: the cheap path that also works logged out.
func NormalizeBackend(b string) string {
	if b == BackendAPI {
		return BackendAPI
	}
	return BackendFree
}

// CatalogItem is a metadata-only record from the YouTube Data API: a video or a
// playlist. It carries no audio — importing one yields a non-mixable Track
// (SourceYouTube), never PCM. For a playlist result, URI is the playlist id,
// Title is the playlist name and Artist is the channel; DurationMs/Album are zero.
type CatalogItem struct {
	URI        string `json:"uri"`
	Title      string `json:"title"`
	Artist     string `json:"artist"` // channel title
	Album      string `json:"album"`  // always "" for YouTube
	ArtworkURL string `json:"artworkUrl"` // thumbnail URL (metadata only)
	DurationMs int    `json:"durationMs"`
	Kind       string `json:"kind"` // KindVideo | KindPlaylist
}

// PendingAuth is the in-flight OAuth attempt held between Begin and the callback:
// State is the CSRF nonce echoed back by Google, Verifier is unused for the
// auth-code-with-secret flow (kept for shape parity with Spotify's PendingAuth),
// and CreatedAt stamps when the flow started (for the TTL).
type PendingAuth struct {
	State     string
	Verifier  string
	CreatedAt time.Time
}

// Expired reports whether the pending flow outlived its TTL by now.
func (p PendingAuth) Expired(now time.Time) bool {
	if p.CreatedAt.IsZero() {
		return true
	}
	return now.Sub(p.CreatedAt) > pendingTTL
}
