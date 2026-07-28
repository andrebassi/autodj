package spotify

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

func newTestOAuth(tokenURL, meURL string) *OAuth {
	return &OAuth{
		ClientID: "cli-id",
		Redirect: "http://localhost:8095/api/v1/spotify/callback",
		HTTP:     http.DefaultClient,
		Now:      fixedClock{time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)},
		authBase: "https://accounts.spotify.com/authorize",
		tokenURL: tokenURL,
		meURL:    meURL,
	}
}

func TestAuthURL(t *testing.T) {
	o := New("cli-id", "http://localhost/cb")
	raw := o.AuthURL("st8", "chg", []string{"streaming", "user-read-private"})
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	q := u.Query()
	if q.Get("client_id") != "cli-id" {
		t.Errorf("client_id = %q", q.Get("client_id"))
	}
	if q.Get("response_type") != "code" {
		t.Errorf("response_type = %q", q.Get("response_type"))
	}
	if q.Get("state") != "st8" {
		t.Errorf("state = %q", q.Get("state"))
	}
	if q.Get("code_challenge") != "chg" || q.Get("code_challenge_method") != "S256" {
		t.Errorf("challenge = %q method %q", q.Get("code_challenge"), q.Get("code_challenge_method"))
	}
	if q.Get("scope") != "streaming user-read-private" {
		t.Errorf("scope = %q", q.Get("scope"))
	}
	if q.Get("redirect_uri") != "http://localhost/cb" {
		t.Errorf("redirect = %q", q.Get("redirect_uri"))
	}
}

func TestNewState(t *testing.T) {
	o := New("c", "r")
	s1, err := o.NewState()
	if err != nil {
		t.Fatalf("NewState: %v", err)
	}
	s2, _ := o.NewState()
	if s1 == "" || s1 == s2 {
		t.Fatalf("state not random: %q %q", s1, s2)
	}
}

func TestNewPKCE(t *testing.T) {
	o := New("c", "r")
	v, ch, err := o.NewPKCE()
	if err != nil {
		t.Fatalf("NewPKCE: %v", err)
	}
	if len(v) < 43 || len(v) > 128 {
		t.Fatalf("verifier length %d out of [43,128]", len(v))
	}
	// challenge must equal base64url(sha256(verifier)).
	sum := sha256.Sum256([]byte(v))
	want := base64.RawURLEncoding.EncodeToString(sum[:])
	if ch != want {
		t.Fatalf("challenge = %q, want %q", ch, want)
	}
	// distinct calls => distinct verifiers.
	v2, _, _ := o.NewPKCE()
	if v == v2 {
		t.Fatal("verifier not random across calls")
	}
}

func TestExchange_ok(t *testing.T) {
	var gotForm url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/token"):
			_ = r.ParseForm()
			gotForm = r.PostForm
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"acc","refresh_token":"ref","expires_in":3600,"scope":"streaming user-read-private"}`))
		case strings.HasSuffix(r.URL.Path, "/me"):
			if r.Header.Get("Authorization") != "Bearer acc" {
				t.Errorf("me auth header = %q", r.Header.Get("Authorization"))
			}
			_, _ = w.Write([]byte(`{"product":"premium"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	o := newTestOAuth(srv.URL+"/token", srv.URL+"/me")
	acc, err := o.Exchange(context.Background(), "code123", "verifier123")
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}
	if gotForm.Get("grant_type") != "authorization_code" || gotForm.Get("code") != "code123" || gotForm.Get("code_verifier") != "verifier123" {
		t.Fatalf("form = %v", gotForm)
	}
	if acc.AccessToken != "acc" || acc.RefreshToken != "ref" || !acc.Premium() {
		t.Fatalf("account = %+v", acc)
	}
	if !acc.HasScope("streaming") || !acc.HasScope("user-read-private") {
		t.Fatalf("scopes = %v", acc.Scopes)
	}
	// ExpiresAt = fixed now + 3600s.
	wantExp := time.Date(2026, 7, 24, 13, 0, 0, 0, time.UTC)
	if !acc.ExpiresAt.Equal(wantExp) {
		t.Fatalf("ExpiresAt = %v, want %v", acc.ExpiresAt, wantExp)
	}
}

func TestExchange_tokenErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()
	o := newTestOAuth(srv.URL+"/token", srv.URL+"/me")
	if _, err := o.Exchange(context.Background(), "c", "v"); err == nil {
		t.Fatal("want error on non-200 token status")
	}
}

func TestExchange_badJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/token") {
			_, _ = w.Write([]byte(`{not json`))
		}
	}))
	defer srv.Close()
	o := newTestOAuth(srv.URL+"/token", srv.URL+"/me")
	if _, err := o.Exchange(context.Background(), "c", "v"); err == nil {
		t.Fatal("want decode error")
	}
}

func TestRefresh_preservesRefreshTokenAndDegradesProduct(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/token"):
			// No refresh_token in response → adapter must keep the old one.
			_, _ = w.Write([]byte(`{"access_token":"acc2","expires_in":3600,"scope":"streaming"}`))
		case strings.HasSuffix(r.URL.Path, "/me"):
			// Profile lookup fails → product degrades to "" (non-premium).
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()
	o := newTestOAuth(srv.URL+"/token", srv.URL+"/me")
	acc, err := o.Refresh(context.Background(), "old-ref")
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if acc.AccessToken != "acc2" {
		t.Fatalf("access = %q", acc.AccessToken)
	}
	if acc.RefreshToken != "old-ref" {
		t.Fatalf("refresh not preserved: %q", acc.RefreshToken)
	}
	if acc.Premium() {
		t.Fatal("product must degrade to non-premium on /me failure")
	}
}

func TestExchange_networkError(t *testing.T) {
	// Point at a closed server to force a transport error.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close()
	o := newTestOAuth(url+"/token", url+"/me")
	if _, err := o.Exchange(context.Background(), "c", "v"); err == nil {
		t.Fatal("want transport error")
	}
}

func TestDefaults(t *testing.T) {
	// Zero-value accessors fall back to defaults.
	o := &OAuth{ClientID: "c", Redirect: "r"}
	if o.httpClient() != http.DefaultClient {
		t.Error("httpClient default")
	}
	if o.now().IsZero() {
		t.Error("now default")
	}
	if o.authURLBase() != defaultAuthBase {
		t.Error("authURLBase default")
	}
	if o.tokenEndpoint() != defaultTokenURL {
		t.Error("tokenEndpoint default")
	}
	if got := o.fetchProduct(context.Background(), "x"); got != "" {
		// real network call — may error to "", acceptable; just ensure no panic.
		_ = got
	}
}

// --- Probe: o diagnóstico (v0.2.26) ---------------------------------------
//
// Diferente de fetchProduct, o Probe NÃO engole status ruim: é justamente o status
// cru que identifica a causa. Um 403 no perfil com token válido é a assinatura de
// conta fora da allowlist de um app em Development Mode — cenário em que o SDK
// conecta, o device registra e o play responde 204, e mesmo assim não sai som.

func TestProbe_perfilEDevicesOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok-1" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/player/devices") {
			_, _ = w.Write([]byte(`{"devices":[{"name":"AutoDJ Playout","is_active":true},{"name":"Celular","is_active":false}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"product":"premium"}`))
	}))
	defer srv.Close()

	o := newTestOAuth(srv.URL+"/token", srv.URL+"/me")
	p, err := o.Probe(context.Background(), "tok-1")
	if err != nil {
		t.Fatal(err)
	}
	if p.ProfileStatus != 200 || p.Product != "premium" {
		t.Fatalf("perfil = %d/%q", p.ProfileStatus, p.Product)
	}
	if p.DevicesStatus != 200 || len(p.Devices) != 2 {
		t.Fatalf("devices = %d/%v", p.DevicesStatus, p.Devices)
	}
	if p.ActiveDevice != "AutoDJ Playout" {
		t.Fatalf("ativo = %q", p.ActiveDevice)
	}
}

func TestProbe_403NoPerfilNaoEhEngolido(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/player/devices") {
			_, _ = w.Write([]byte(`{"devices":[]}`))
			return
		}
		w.WriteHeader(http.StatusForbidden) // corpo vazio, como o Spotify responde
	}))
	defer srv.Close()

	o := newTestOAuth(srv.URL+"/token", srv.URL+"/me")
	p, err := o.Probe(context.Background(), "tok-1")
	if err != nil {
		t.Fatal(err)
	}
	if p.ProfileStatus != 403 {
		t.Fatalf("ProfileStatus = %d, want 403 preservado (é o sinal da allowlist)", p.ProfileStatus)
	}
	if p.Product != "" {
		t.Fatalf("Product = %q, want vazio quando o perfil não responde", p.Product)
	}
}

func TestProbe_401TokenMorto(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	o := newTestOAuth(srv.URL+"/token", srv.URL+"/me")
	p, _ := o.Probe(context.Background(), "expirado")
	if p.ProfileStatus != 401 || p.DevicesStatus != 401 {
		t.Fatalf("statuses = %d/%d, want 401/401", p.ProfileStatus, p.DevicesStatus)
	}
}

func TestProbe_redeForaNaoQuebra(t *testing.T) {
	o := newTestOAuth("http://127.0.0.1:1/token", "http://127.0.0.1:1/me")
	p, err := o.Probe(context.Background(), "tok")
	if err != nil {
		t.Fatalf("Probe não deve propagar erro de rede: %v", err)
	}
	if p.ProfileStatus != 0 || p.DevicesStatus != 0 {
		t.Fatalf("statuses = %d/%d, want 0/0 (indisponível)", p.ProfileStatus, p.DevicesStatus)
	}
}
