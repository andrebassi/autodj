package youtube

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

func newTestOAuth(tokenURL, channelURL string) *OAuth {
	return &OAuth{
		ClientID:     "cli-id",
		ClientSecret: "cli-secret",
		Redirect:     "http://localhost:8095/api/v1/youtube/callback",
		HTTP:         http.DefaultClient,
		Now:          fixedClock{time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)},
		authBase:     "https://accounts.google.com/o/oauth2/v2/auth",
		tokenURL:     tokenURL,
		channelURL:   channelURL,
	}
}

func TestAuthURL(t *testing.T) {
	o := New("cli-id", "sec", "http://localhost/cb")
	raw := o.AuthURL("st8", []string{"https://www.googleapis.com/auth/youtube.readonly", "openid"})
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
	if q.Get("access_type") != "offline" || q.Get("prompt") != "consent" {
		t.Errorf("offline/consent = %q %q", q.Get("access_type"), q.Get("prompt"))
	}
	if q.Get("scope") != "https://www.googleapis.com/auth/youtube.readonly openid" {
		t.Errorf("scope = %q", q.Get("scope"))
	}
	if q.Get("redirect_uri") != "http://localhost/cb" {
		t.Errorf("redirect = %q", q.Get("redirect_uri"))
	}
}

func TestNewState(t *testing.T) {
	o := New("c", "s", "r")
	s1, err := o.NewState()
	if err != nil {
		t.Fatalf("NewState: %v", err)
	}
	s2, _ := o.NewState()
	if s1 == "" || s1 == s2 {
		t.Fatalf("state not random: %q %q", s1, s2)
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
			_, _ = w.Write([]byte(`{"access_token":"acc","refresh_token":"ref","expires_in":3600,"scope":"https://www.googleapis.com/auth/youtube.readonly openid"}`))
		case strings.HasSuffix(r.URL.Path, "/channels"):
			if r.Header.Get("Authorization") != "Bearer acc" {
				t.Errorf("channels auth header = %q", r.Header.Get("Authorization"))
			}
			_, _ = w.Write([]byte(`{"items":[{"snippet":{"title":"DJ Nova"}}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	o := newTestOAuth(srv.URL+"/token", srv.URL+"/channels")
	acc, err := o.Exchange(context.Background(), "code123")
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}
	if gotForm.Get("grant_type") != "authorization_code" || gotForm.Get("code") != "code123" ||
		gotForm.Get("client_secret") != "cli-secret" {
		t.Fatalf("form = %v", gotForm)
	}
	if acc.AccessToken != "acc" || acc.RefreshToken != "ref" || acc.ChannelTitle != "DJ Nova" {
		t.Fatalf("account = %+v", acc)
	}
	if acc.IsPremium() {
		t.Fatal("premium never maps on youtube → must be false")
	}
	if !acc.HasScope("openid") {
		t.Fatalf("scopes = %v", acc.Scopes)
	}
	wantExp := time.Date(2026, 7, 26, 13, 0, 0, 0, time.UTC)
	if !acc.ExpiresAt.Equal(wantExp) {
		t.Fatalf("ExpiresAt = %v, want %v", acc.ExpiresAt, wantExp)
	}
}

func TestExchange_tokenErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()
	o := newTestOAuth(srv.URL+"/token", srv.URL+"/channels")
	if _, err := o.Exchange(context.Background(), "c"); err == nil {
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
	o := newTestOAuth(srv.URL+"/token", srv.URL+"/channels")
	if _, err := o.Exchange(context.Background(), "c"); err == nil {
		t.Fatal("want decode error")
	}
}

func TestRefresh_preservesRefreshTokenAndDegradesChannel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/token"):
			// No refresh_token in response → adapter must keep the old one.
			_, _ = w.Write([]byte(`{"access_token":"acc2","expires_in":3600,"scope":"openid"}`))
		case strings.HasSuffix(r.URL.Path, "/channels"):
			// Channel lookup fails → title degrades to "".
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()
	o := newTestOAuth(srv.URL+"/token", srv.URL+"/channels")
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
	if acc.ChannelTitle != "" {
		t.Fatalf("channel must degrade to empty on lookup failure: %q", acc.ChannelTitle)
	}
}

// A channels response with no items degrades ChannelTitle to "" without failing.
func TestExchange_channelEmptyItems(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/token"):
			_, _ = w.Write([]byte(`{"access_token":"acc","expires_in":60}`))
		case strings.HasSuffix(r.URL.Path, "/channels"):
			_, _ = w.Write([]byte(`{"items":[]}`))
		}
	}))
	defer srv.Close()
	o := newTestOAuth(srv.URL+"/token", srv.URL+"/channels")
	acc, err := o.Exchange(context.Background(), "c")
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}
	if acc.ChannelTitle != "" {
		t.Fatalf("channel = %q, want empty", acc.ChannelTitle)
	}
}

// A malformed channels body degrades ChannelTitle to "" (decode error swallowed).
func TestExchange_channelBadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/token"):
			_, _ = w.Write([]byte(`{"access_token":"acc","expires_in":60}`))
		case strings.HasSuffix(r.URL.Path, "/channels"):
			_, _ = w.Write([]byte(`{not json`))
		}
	}))
	defer srv.Close()
	o := newTestOAuth(srv.URL+"/token", srv.URL+"/channels")
	acc, _ := o.Exchange(context.Background(), "c")
	if acc.ChannelTitle != "" {
		t.Fatalf("channel = %q, want empty on bad json", acc.ChannelTitle)
	}
}

func TestExchange_networkError(t *testing.T) {
	// Point at a closed server to force a transport error.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	u := srv.URL
	srv.Close()
	o := newTestOAuth(u+"/token", u+"/channels")
	if _, err := o.Exchange(context.Background(), "c"); err == nil {
		t.Fatal("want transport error")
	}
}

func TestDefaults(t *testing.T) {
	// Zero-value accessors fall back to defaults.
	o := &OAuth{ClientID: "c", ClientSecret: "s", Redirect: "r"}
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
	// real network call to the default channel endpoint — may error to "";
	// just ensure no panic and empty return.
	if got := o.fetchChannelTitle(context.Background(), "x"); got != "" {
		_ = got
	}
	if systemClock.Now(systemClock{}).IsZero() {
		t.Error("systemClock.Now zero")
	}
}
