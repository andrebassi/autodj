// Package youtube implements the outbound.YouTubeOAuth and outbound.YouTubeCatalog
// ports against Google's OAuth endpoints and the YouTube Data API v3. OAuth uses
// the Authorization Code flow WITH a client secret (a confidential client — the
// secret comes from the environment/pass, never the binary source), NOT PKCE like
// Spotify. It creates no Track: the two-engine split (youtube tracks are
// non-mixable) is enforced downstream when import turns metadata into tracks.
package youtube

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	domyoutube "autodj/domain/youtube"
)

// defaultAuthBase / defaultTokenURL / defaultChannelURL are Google's public
// endpoints. They are overridable so httptest can point the exchange at a local
// server.
const (
	defaultAuthBase   = "https://accounts.google.com/o/oauth2/v2/auth"
	defaultTokenURL   = "https://oauth2.googleapis.com/token"
	defaultChannelURL = "https://www.googleapis.com/youtube/v3/channels?part=snippet&mine=true"
)

// Clock abstracts time so token expiry (ExpiresAt = now + expires_in) is testable.
type Clock interface{ Now() time.Time }

// OAuth is the accounts.google.com client. ClientID/ClientSecret identify the
// confidential app; Redirect is the registered callback URL. HTTP, Now, authBase,
// tokenURL and channelURL are injectable for tests; zero values fall back to sane
// defaults.
type OAuth struct {
	ClientID     string
	ClientSecret string
	Redirect     string
	HTTP         *http.Client
	Now          Clock
	authBase     string
	tokenURL     string
	channelURL   string
}

// New builds an OAuth client for the given confidential client id/secret and
// redirect URL.
func New(clientID, clientSecret, redirect string) *OAuth {
	return &OAuth{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Redirect:     redirect,
		HTTP:         &http.Client{Timeout: 15 * time.Second},
		Now:          systemClock{},
		authBase:     defaultAuthBase,
		tokenURL:     defaultTokenURL,
		channelURL:   defaultChannelURL,
	}
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

func (o *OAuth) httpClient() *http.Client {
	if o.HTTP != nil {
		return o.HTTP
	}
	return http.DefaultClient
}

func (o *OAuth) now() time.Time {
	if o.Now != nil {
		return o.Now.Now()
	}
	return time.Now()
}

func (o *OAuth) authURLBase() string {
	if o.authBase != "" {
		return o.authBase
	}
	return defaultAuthBase
}

func (o *OAuth) tokenEndpoint() string {
	if o.tokenURL != "" {
		return o.tokenURL
	}
	return defaultTokenURL
}

// AuthURL builds the authorize URL: response_type=code, state, redirect, scopes,
// access_type=offline + prompt=consent so Google issues a refresh token on the
// first (and subsequent) consent.
func (o *OAuth) AuthURL(state string, scopes []string) string {
	q := url.Values{}
	q.Set("client_id", o.ClientID)
	q.Set("response_type", "code")
	q.Set("redirect_uri", o.Redirect)
	q.Set("state", state)
	q.Set("scope", strings.Join(scopes, " "))
	q.Set("access_type", "offline")
	q.Set("prompt", "consent")
	q.Set("include_granted_scopes", "true")
	return o.authURLBase() + "?" + q.Encode()
}

// NewState returns a random URL-safe CSRF state nonce.
func (o *OAuth) NewState() (string, error) {
	return randomURLSafe(32)
}

// Exchange trades an authorization code for an Account, presenting the client
// secret (confidential client, no PKCE).
func (o *OAuth) Exchange(ctx context.Context, code string) (domyoutube.Account, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", o.Redirect)
	form.Set("client_id", o.ClientID)
	form.Set("client_secret", o.ClientSecret)
	return o.postToken(ctx, form, "")
}

// Refresh trades a refresh token for a fresh Account. Google may omit a new
// refresh token — the old one is preserved.
func (o *OAuth) Refresh(ctx context.Context, refreshToken string) (domyoutube.Account, error) {
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)
	form.Set("client_id", o.ClientID)
	form.Set("client_secret", o.ClientSecret)
	return o.postToken(ctx, form, refreshToken)
}

// tokenResponse is the oauth2.googleapis.com/token payload.
type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	Scope        string `json:"scope"`
}

// postToken posts the form to the token endpoint and maps the response to an
// Account. prevRefresh is preserved when the response omits a refresh token.
func (o *OAuth) postToken(ctx context.Context, form url.Values, prevRefresh string) (domyoutube.Account, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.tokenEndpoint(), strings.NewReader(form.Encode()))
	if err != nil {
		return domyoutube.Account{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := o.httpClient().Do(req)
	if err != nil {
		return domyoutube.Account{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return domyoutube.Account{}, fmt.Errorf("youtube: token endpoint status %d", resp.StatusCode)
	}
	var tr tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return domyoutube.Account{}, fmt.Errorf("youtube: decode token response: %w", err)
	}

	refresh := tr.RefreshToken
	if refresh == "" {
		refresh = prevRefresh
	}
	var scopes []string
	if tr.Scope != "" {
		scopes = strings.Fields(tr.Scope)
	}
	return domyoutube.Account{
		AccessToken:  tr.AccessToken,
		RefreshToken: refresh,
		ExpiresAt:    o.now().Add(time.Duration(tr.ExpiresIn) * time.Second),
		Scopes:       scopes,
		// ChannelTitle comes from the channels.list?mine=true lookup so the panel
		// can show which channel is linked. A failed lookup degrades to empty
		// rather than failing the whole exchange. Premium never maps cleanly on
		// YouTube and playout does not depend on it → always false (best-effort).
		ChannelTitle: o.fetchChannelTitle(ctx, tr.AccessToken),
		Premium:      false,
	}, nil
}

// channelsResponse is the subset of channels.list the OAuth flow needs.
type channelsResponse struct {
	Items []struct {
		Snippet struct {
			Title string `json:"title"`
		} `json:"snippet"`
	} `json:"items"`
}

// fetchChannelTitle reads the linked channel's title from channels.list?mine=true.
// Any error (network, non-200, decode, empty) degrades to "" — the connection
// still succeeds; only the display title is unknown.
func (o *OAuth) fetchChannelTitle(ctx context.Context, accessToken string) string {
	endpoint := o.channelURL
	if endpoint == "" {
		endpoint = defaultChannelURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := o.httpClient().Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	var cr channelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil {
		return ""
	}
	if len(cr.Items) == 0 {
		return ""
	}
	return cr.Items[0].Snippet.Title
}

// randomURLSafe returns n random bytes as a base64url (no padding) string.
func randomURLSafe(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
