// Package spotify models the OAuth session that links a home operator to their
// Spotify account. It is pure domain: no network, no crypto, no persistence —
// the PKCE verifier/challenge are generated in the adapter and the token
// exchange happens over the outbound port. The two-engine split lives elsewhere
// (a Spotify Track is always non-mixable via the source matrix); this package
// only tracks the account lifecycle so the panel can leave the stub behind.
package spotify

import (
	"errors"
	"time"
)

// refreshWindow is the slack before ExpiresAt at which a token is considered
// stale enough to refresh proactively, so AccessToken never hands out a token
// about to die mid-request.
const refreshWindow = 60 * time.Second

// pendingTTL bounds how long a started auth flow stays valid: the operator must
// return from the Spotify consent screen within this window or restart.
const pendingTTL = 10 * time.Minute

// productPremium is the Spotify product string that unlocks the Web Playback
// SDK (Story 6.3). Anything else (free/open) is treated as non-Premium.
const productPremium = "premium"

// Domain errors surfaced by the app service and mapped to HTTP status upstream.
var (
	// ErrNoAccount means no Spotify account is linked (the repo is empty).
	ErrNoAccount = errors.New("spotify: no account connected")
	// ErrStateMismatch means the callback state did not match the pending flow
	// (CSRF guard) — the callback is rejected.
	ErrStateMismatch = errors.New("spotify: oauth state mismatch")
	// ErrAuthExpired means the pending auth flow outlived its TTL before the
	// callback arrived.
	ErrAuthExpired = errors.New("spotify: pending auth expired")
	// ErrNotPremium means the linked account lacks Premium (Web Playback SDK is
	// unavailable). Kept here for 6.3; 6.1 only reports the flag.
	ErrNotPremium = errors.New("spotify: account is not premium")
	// ErrCatalogUnavailable means the Web API catalog gateway (search/import,
	// Story 6.2) is not wired — OAuth works but no catalog client is configured.
	ErrCatalogUnavailable = errors.New("spotify: catalog gateway unavailable")
)

// Probe é o retrato do que o Spotify responde AGORA para a sessão guardada. Existe
// por causa de uma caçada de horas: o Spotify pode aceitar `connect`, registrar o
// device e responder 204 ao comando de tocar — sem UM evento de erro — e mesmo
// assim não emitir som. O sinal que denunciava a causa (conta fora da allowlist de
// um app em Development Mode) era um 403 no `/v1/me` chamado PELO SERVIDOR.
//
// Guardar o status HTTP cru, e não só um booleano, é proposital: 403 e 401 têm
// causas e soluções diferentes (allowlist vs token morto) e a UI precisa dizer qual.
type Probe struct {
	// ProfileStatus é o HTTP de GET /v1/me. 200 = conta liberada; 403 = fora da
	// allowlist (Development Mode); 401 = token inválido/expirado.
	ProfileStatus int
	// Product vem do perfil quando ele responde ("premium", "free", "open").
	Product string
	// DevicesStatus é o HTTP de GET /v1/me/player/devices.
	DevicesStatus int
	// Devices são os nomes dos aparelhos visíveis; ActiveDevice é o que está no ar.
	Devices      []string
	ActiveDevice string
}

// Account is the persisted OAuth session for the linked Spotify user. Tokens are
// opaque strings from Spotify; ExpiresAt is when the access token dies. Scopes
// records what was granted so callers can gate features without a round-trip.
type Account struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
	Scopes       []string
	// Product is the Spotify subscription tier ("premium", "free", "open").
	Product string
}

// Connected reports whether an account is actually linked (has an access token).
// The zero Account is "not connected".
func (a Account) Connected() bool { return a.AccessToken != "" }

// Premium reports whether the account can drive the Web Playback SDK.
func (a Account) Premium() bool { return a.Product == productPremium }

// Expired reports whether the access token is dead at now (inclusive of the
// exact instant). A zero ExpiresAt is treated as expired (no valid token yet).
func (a Account) Expired(now time.Time) bool {
	if a.ExpiresAt.IsZero() {
		return true
	}
	return !now.Before(a.ExpiresAt)
}

// NeedsRefresh reports whether the token is expired or within the refresh window
// of expiring, so AccessToken refreshes proactively. Requires a refresh token to
// be actionable, but the time check stands on its own for callers that inspect it.
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

// Catalog search kinds. A search targets tracks or playlists; import expands a
// playlist into its tracks. Kept here so both ports agree on the vocabulary.
const (
	KindTrack    = "track"
	KindPlaylist = "playlist"
)

// CatalogItem is a metadata-only record from the Spotify Web API: a track or a
// playlist. It carries no audio — importing one yields a non-mixable Track
// (SourceSpotify), never PCM. For a playlist result, URI is spotify:playlist:…,
// Title is the playlist name and Artist is the owner; DurationMs/Album are zero.
type CatalogItem struct {
	URI        string `json:"uri"`
	Title      string `json:"title"`
	Artist     string `json:"artist"`
	Album      string `json:"album"`
	ArtworkURL string `json:"artworkUrl"` // album cover image URL (metadata only)
	DurationMs int    `json:"durationMs"`
	Kind       string `json:"kind"` // KindTrack | KindPlaylist
}

// PendingAuth is the in-flight OAuth attempt held between Begin and the callback:
// State is the CSRF nonce echoed back by Spotify, Verifier is the PKCE
// code_verifier the adapter must present at the token exchange, and CreatedAt
// stamps when the flow started (for the TTL).
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
