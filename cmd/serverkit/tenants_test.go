package serverkit

import (
	"net/http/httptest"
	"testing"
)

func TestSanitizeSession(t *testing.T) {
	cases := map[string]string{
		"":                             defaultTenant,
		"   ":                          defaultTenant,
		"!!!":                          defaultTenant,
		"AbC123":                       "abc123",
		"3f8a-9b2c/../etc":             "3f8a9b2cetc", // path traversal stripped
		"a1b2c3d4e5f6a7b8c9d0e1f2a3b4": "a1b2c3d4e5f6a7b8c9d0e1f2a3b4",
	}
	for in, want := range cases {
		if got := sanitizeSession(in); got != want {
			t.Errorf("sanitizeSession(%q) = %q, want %q", in, got, want)
		}
	}
	// over-length is truncated to maxSessionLen
	long := ""
	for i := 0; i < 200; i++ {
		long += "a"
	}
	if got := sanitizeSession(long); len(got) != maxSessionLen {
		t.Errorf("sanitizeSession(long) len = %d, want %d", len(got), maxSessionLen)
	}
}

func TestSessionID(t *testing.T) {
	// header wins for REST
	r := httptest.NewRequest("GET", "/api/v1/playback/queue", nil)
	r.Header.Set(sessionHeader, "Browser-1")
	if got := sessionID(r); got != "browser1" {
		t.Errorf("header session = %q, want %q", got, "browser1")
	}
	// ?sid= for the WebSocket handshake (no custom headers there)
	r = httptest.NewRequest("GET", "/ws?sid=Browser-2", nil)
	if got := sessionID(r); got != "browser2" {
		t.Errorf("query session = %q, want %q", got, "browser2")
	}
	// header takes precedence over query when both present
	r = httptest.NewRequest("GET", "/ws?sid=fromquery", nil)
	r.Header.Set(sessionHeader, "fromheader")
	if got := sessionID(r); got != "fromheader" {
		t.Errorf("precedence session = %q, want %q", got, "fromheader")
	}
	// neither → shared default tenant
	r = httptest.NewRequest("GET", "/api/v1/tracks", nil)
	if got := sessionID(r); got != defaultTenant {
		t.Errorf("absent session = %q, want %q", got, defaultTenant)
	}
}

func TestSessionFromState(t *testing.T) {
	// smuggled envelope: "<sid>~<rawstate>" → sid extracted, raw state returned intact
	rawState := "aB3-_xYz09" // RawURLEncoding alphabet, no '~'
	r := httptest.NewRequest("GET", "/api/v1/spotify/callback?code=xyz&state=abc123~"+rawState, nil)
	sid, raw, ok := sessionFromState(r)
	if !ok || sid != "abc123" || raw != rawState {
		t.Errorf("sessionFromState = (%q,%q,%v), want (abc123,%q,true)", sid, raw, ok, rawState)
	}
	// default-sid envelope: a browser whose FingerprintJS hasn't resolved uses sid
	// "default". The '~' MUST still be peeled so Complete() gets the raw state — else
	// "default~<state>" stays glued and OAuth fails with a state mismatch (regression).
	r = httptest.NewRequest("GET", "/api/v1/youtube/callback?code=xyz&state=default~"+rawState, nil)
	if sid, raw, ok := sessionFromState(r); !ok || sid != "default" || raw != rawState {
		t.Errorf("default envelope = (%q,%q,%v), want (default,%q,true)", sid, raw, ok, rawState)
	}
	// plain state (no delimiter) → not ours, default routing
	r = httptest.NewRequest("GET", "/api/v1/spotify/callback?state="+rawState, nil)
	if _, _, ok := sessionFromState(r); ok {
		t.Errorf("plain state must not match envelope")
	}
	// leading delimiter (empty sid) → ignored
	r = httptest.NewRequest("GET", "/cb?state=~"+rawState, nil)
	if _, _, ok := sessionFromState(r); ok {
		t.Errorf("empty sid prefix must not match")
	}
	// no state at all → not ours
	r = httptest.NewRequest("GET", "/cb", nil)
	if _, _, ok := sessionFromState(r); ok {
		t.Errorf("absent state must not match")
	}
}
