// Package spotify implements the outbound.SpotifyOAuth port against
// accounts.spotify.com using the OAuth 2.0 Authorization Code + PKCE flow for a
// public client (no client secret — home use, the client id is public and the
// secret never ships in the binary). PKCE crypto (verifier/challenge) lives here,
// never in the domain. It creates no Track: the two-engine split (spotify tracks
// are non-mixable) is enforced downstream when 6.2 imports metadata.
package spotify

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	domspotify "autodj/domain/spotify"
)

// defaultAuthBase and defaultTokenURL are Spotify's public OAuth endpoints. They
// are overridable so httptest can point the token exchange at a local server.
const (
	defaultAuthBase  = "https://accounts.spotify.com/authorize"
	defaultTokenURL  = "https://accounts.spotify.com/api/token"
	defaultMeURL     = "https://api.spotify.com/v1/me"
	verifierBytesLen = 64 // 64 random bytes → 86-char base64url verifier (in [43,128]).
)

// Clock abstracts time so token expiry (ExpiresAt = now + expires_in) is testable.
type Clock interface{ Now() time.Time }

// OAuth is the accounts.spotify.com client. ClientID is the public app id;
// Redirect is the registered callback URL. HTTP, Now, authBase and tokenURL are
// injectable for tests; zero values fall back to sane defaults.
type OAuth struct {
	ClientID string
	Redirect string
	HTTP     *http.Client
	Now      Clock
	authBase string
	tokenURL string
	meURL    string
}

// New builds an OAuth client for the given public client id and redirect URL.
func New(clientID, redirect string) *OAuth {
	return &OAuth{
		ClientID: clientID,
		Redirect: redirect,
		HTTP:     &http.Client{Timeout: 15 * time.Second},
		Now:      systemClock{},
		authBase: defaultAuthBase,
		tokenURL: defaultTokenURL,
		meURL:    defaultMeURL,
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

// AuthURL builds the authorize URL: response_type=code, PKCE S256 challenge,
// state, redirect and the requested scopes.
func (o *OAuth) AuthURL(state, challenge string, scopes []string) string {
	q := url.Values{}
	q.Set("client_id", o.ClientID)
	q.Set("response_type", "code")
	q.Set("redirect_uri", o.Redirect)
	q.Set("state", state)
	q.Set("scope", strings.Join(scopes, " "))
	q.Set("code_challenge_method", "S256")
	q.Set("code_challenge", challenge)
	return o.authURLBase() + "?" + q.Encode()
}

// NewState returns a random URL-safe CSRF state nonce.
func (o *OAuth) NewState() (string, error) {
	return randomURLSafe(32)
}

// NewPKCE returns a fresh (verifier, challenge) pair: verifier is a high-entropy
// base64url string in [43,128] chars, challenge = base64url(sha256(verifier)).
func (o *OAuth) NewPKCE() (string, string, error) {
	verifier, err := randomURLSafe(verifierBytesLen)
	if err != nil {
		return "", "", err
	}
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	return verifier, challenge, nil
}

// Exchange trades an authorization code (+ PKCE verifier) for an Account.
func (o *OAuth) Exchange(ctx context.Context, code, verifier string) (domspotify.Account, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", o.Redirect)
	form.Set("client_id", o.ClientID)
	form.Set("code_verifier", verifier)
	return o.postToken(ctx, form, "")
}

// Refresh trades a refresh token for a fresh Account. Spotify may omit a new
// refresh token — the old one is preserved.
func (o *OAuth) Refresh(ctx context.Context, refreshToken string) (domspotify.Account, error) {
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)
	form.Set("client_id", o.ClientID)
	return o.postToken(ctx, form, refreshToken)
}

// tokenResponse is the accounts.spotify.com/api/token payload.
type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	Scope        string `json:"scope"`
}

// postToken posts the form to the token endpoint and maps the response to an
// Account. prevRefresh is preserved when the response omits a refresh token.
func (o *OAuth) postToken(ctx context.Context, form url.Values, prevRefresh string) (domspotify.Account, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.tokenEndpoint(), strings.NewReader(form.Encode()))
	if err != nil {
		return domspotify.Account{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := o.httpClient().Do(req)
	if err != nil {
		return domspotify.Account{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return domspotify.Account{}, fmt.Errorf("spotify: token endpoint status %d", resp.StatusCode)
	}
	var tr tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return domspotify.Account{}, fmt.Errorf("spotify: decode token response: %w", err)
	}

	refresh := tr.RefreshToken
	if refresh == "" {
		refresh = prevRefresh
	}
	var scopes []string
	if tr.Scope != "" {
		scopes = strings.Fields(tr.Scope)
	}
	return domspotify.Account{
		AccessToken:  tr.AccessToken,
		RefreshToken: refresh,
		ExpiresAt:    o.now().Add(time.Duration(tr.ExpiresIn) * time.Second),
		Scopes:       scopes,
		// Product comes from the /me profile lookup so Premium() reflects the real
		// subscription (needed by 6.3's Web Playback SDK gate). A failed lookup
		// degrades to empty (non-premium) rather than failing the whole exchange.
		Product: o.fetchProduct(ctx, tr.AccessToken),
	}, nil
}

// profileResponse is the subset of GET /v1/me the OAuth flow needs.
type profileResponse struct {
	Product string `json:"product"`
}

// fetchProduct reads the account's subscription tier from GET /v1/me. Any error
// (network, non-200, decode) degrades to "" (non-premium) — the connection still
// succeeds; only the Premium flag is unknown.
func (o *OAuth) fetchProduct(ctx context.Context, accessToken string) string {
	endpoint := o.meURL
	if endpoint == "" {
		endpoint = defaultMeURL
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
	var pr profileResponse
	if err := json.NewDecoder(resp.Body).Decode(&pr); err != nil {
		return ""
	}
	return pr.Product
}

// devicesResponse é o subconjunto de GET /v1/me/player/devices que o diagnóstico usa.
type devicesResponse struct {
	Devices []struct {
		Name     string `json:"name"`
		IsActive bool   `json:"is_active"`
	} `json:"devices"`
}

// Probe pergunta ao Spotify o que ele acha da sessão AGORA — perfil e devices — e
// devolve os status HTTP crus, sem interpretar.
//
// Diferente de fetchProduct, aqui um não-200 NÃO é engolido: é justamente o dado
// que interessa. Um 403 no perfil com o token válido é a assinatura de conta fora
// da allowlist de um app em Development Mode, que é indistinguível de "tudo certo"
// por qualquer outro caminho — o SDK conecta, o device registra e o play responde
// 204 mesmo assim.
func (o *OAuth) Probe(ctx context.Context, accessToken string) (domspotify.Probe, error) {
	var p domspotify.Probe

	meURL := o.meURL
	if meURL == "" {
		meURL = defaultMeURL
	}
	p.ProfileStatus, _ = o.probeGet(ctx, meURL, accessToken, func(dec *json.Decoder) {
		var pr profileResponse
		if dec.Decode(&pr) == nil {
			p.Product = pr.Product
		}
	})

	// devices fica no mesmo host do perfil (api.spotify.com/v1) — derivar daí mantém
	// os testes apontando p/ um httptest só.
	p.DevicesStatus, _ = o.probeGet(ctx, meURL+"/player/devices", accessToken, func(dec *json.Decoder) {
		var dr devicesResponse
		if dec.Decode(&dr) == nil {
			for _, d := range dr.Devices {
				p.Devices = append(p.Devices, d.Name)
				if d.IsActive {
					p.ActiveDevice = d.Name
				}
			}
		}
	})
	return p, nil
}

// probeGet faz um GET autenticado e devolve o status; chama onOK só em 200.
func (o *OAuth) probeGet(ctx context.Context, url, token string, onOK func(*json.Decoder)) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := o.httpClient().Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK && onOK != nil {
		onOK(json.NewDecoder(resp.Body))
	}
	return resp.StatusCode, nil
}

// randomURLSafe returns n random bytes as a base64url (no padding) string.
func randomURLSafe(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
