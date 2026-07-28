package serverkit

import (
	"context"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// Per-browser isolation: each distinct browser fingerprint (a FingerprintJS
// visitorId minted client-side and cached in localStorage) resolves to its own
// private backend stack — separate SQLite file, repos, playback engine, WebSocket
// hub and background workers. The frontend sends the id in the X-AutoDJ-Session
// header on REST calls and as ?sid= on the WebSocket handshake (browsers can't set
// custom headers on a WS upgrade). A request with no id falls back to the shared
// "default" tenant so curl/tests/first paint keep working.

const (
	sessionHeader = "X-AutoDJ-Session"
	sessionQuery  = "sid"
	defaultTenant = "default"
	maxSessionLen = 64
)

// sessionInvalid strips everything but [a-z0-9]. A FingerprintJS visitorId is a
// hex-ish string; sanitising to that alphabet both normalises it and makes it safe
// as a filename component (no path traversal into the sessions dir).
var sessionInvalid = regexp.MustCompile(`[^a-z0-9]+`)

// sanitizeSession normalises a raw session id to a safe filename token. Empty or
// fully-invalid input collapses to the shared default tenant.
func sanitizeSession(raw string) string {
	s := sessionInvalid.ReplaceAllString(strings.ToLower(strings.TrimSpace(raw)), "")
	if s == "" {
		return defaultTenant
	}
	if len(s) > maxSessionLen {
		s = s[:maxSessionLen]
	}
	return s
}

// sessionID extracts the browser fingerprint: the header for REST, the ?sid= query
// for the WebSocket. Absent on both → the shared default tenant.
func sessionID(r *http.Request) string {
	if v := r.Header.Get(sessionHeader); v != "" {
		return sanitizeSession(v)
	}
	if v := r.URL.Query().Get(sessionQuery); v != "" {
		return sanitizeSession(v)
	}
	return defaultTenant
}

// stateSep splits the smuggled session from the raw OAuth state. Spotify round-trips
// the `state` param verbatim; the RawURLEncoding alphabet (A-Za-z0-9-_) never contains
// '~', so it's a collision-free delimiter for the "<sid>~<state>" envelope.
const stateSep = '~'

// sessionFromState recovers the fingerprint smuggled in the OAuth `state` param. The
// callback (GET /spotify/callback) is a top-level browser navigation redirected from
// Spotify/Google — it carries no X-AutoDJ-Session header and no ?sid=, so without this
// it would land on the default tenant with the WRONG state and Complete() would fail.
// The frontend rewrites state to "<sid>~<state>" before authorising; here we peel the
// sid back off and return the raw state so the handler validates it unchanged.
//
// The envelope is ALWAYS peeled when a '~' is present — including "default~<state>"
// (the browser whose FingerprintJS hasn't resolved yet uses sid "default"). Skipping
// the peel for the default sid would leave the "default~" prefix glued to the state
// and Complete() would reject it as an oauth state mismatch. sanitizeSession collapses
// an empty/invalid sid to "default", so the returned sid always routes somewhere valid.
func sessionFromState(r *http.Request) (sid, raw string, ok bool) {
	st := r.URL.Query().Get("state")
	i := strings.IndexByte(st, stateSep)
	if i <= 0 {
		return "", "", false
	}
	return sanitizeSession(st[:i]), st[i+1:], true
}

// tenant is one lazily-built per-fingerprint stack. ready is closed once the build
// finishes (success or error) so concurrent first-hits on the same id wait instead
// of building the heavy stack twice.
type tenant struct {
	handler http.Handler
	cleanup func()
	err     error
	ready   chan struct{}
}

// tenantMux fronts the per-fingerprint stacks: it maps a session id to its cached
// stack, building one on first contact. Different sessions build concurrently; only
// repeated cold-starts of the SAME id serialise (via the tenant's ready channel).
type tenantMux struct {
	ctx  context.Context
	base string // sessions dir holding per-tenant SQLite files
	mem  bool   // AUTODJ_DB_PATH=memory → per-tenant ephemeral repo instead of a file

	mu      sync.Mutex
	tenants map[string]*tenant
}

// newTenantMux prepares the dispatcher. The sessions dir (AUTODJ_SESSIONS_DIR, else
// ~/.autodj/sessions) is created eagerly unless the repo is in-memory.
func newTenantMux(ctx context.Context) *tenantMux {
	base := envOr("AUTODJ_SESSIONS_DIR", defaultSessionsDir())
	db := os.Getenv("AUTODJ_DB_PATH")
	mem := db == "memory" || db == ":memory:"
	if !mem {
		if err := os.MkdirAll(base, 0o755); err != nil {
			log.Printf("tenant: cannot create sessions dir %q: %v", base, err)
		}
	}
	return &tenantMux{ctx: ctx, base: base, mem: mem, tenants: map[string]*tenant{}}
}

// dbPathFor is the SQLite file backing a session (or "memory" in ephemeral mode).
// The shared default tenant (no fingerprint — curl/tests/first paint) keeps the
// legacy per-user file so a pre-fingerprint home library survives the upgrade;
// real browser fingerprints get their own file under the sessions dir.
func (t *tenantMux) dbPathFor(id string) string {
	if t.mem {
		return "memory"
	}
	if id == defaultTenant {
		return defaultDBPath()
	}
	return filepath.Join(t.base, id+".db")
}

// resolve returns the (cached or freshly built) handler for a session id.
func (t *tenantMux) resolve(id string) (http.Handler, error) {
	t.mu.Lock()
	tn := t.tenants[id]
	if tn != nil {
		t.mu.Unlock()
		<-tn.ready
		return tn.handler, tn.err
	}
	tn = &tenant{ready: make(chan struct{})}
	t.tenants[id] = tn
	t.mu.Unlock()

	// Every tenant (default OR real fingerprint) builds an EMPTY station — no demo
	// seeding. The operator fills it (scan folders, import Spotify/YouTube).
	h, cl, err := buildTenant(t.ctx, id, t.dbPathFor(id))
	tn.handler, tn.cleanup, tn.err = h, cl, err
	if err != nil {
		// drop the failed entry so a later request retries the build
		t.mu.Lock()
		delete(t.tenants, id)
		t.mu.Unlock()
	} else {
		log.Printf("tenant: built stack for session %q (%s)", id, t.dbPathFor(id))
	}
	close(tn.ready)
	return tn.handler, tn.err
}

func (t *tenantMux) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id := sessionID(r)
	// OAuth callback: no header/sid, session smuggled in `state`. Route to that
	// tenant and restore the raw state so downstream Complete() validates unchanged.
	if id == defaultTenant {
		if sid, raw, ok := sessionFromState(r); ok {
			id = sid
			q := r.URL.Query()
			q.Set("state", raw)
			r.URL.RawQuery = q.Encode()
		}
	}
	h, err := t.resolve(id)
	if err != nil {
		log.Printf("tenant %q: build failed: %v", id, err)
		http.Error(w, "session backend unavailable", http.StatusServiceUnavailable)
		return
	}
	h.ServeHTTP(w, r)
}

// Close tears every open tenant stack down (closes its DB, stops its analysis
// worker). Background loops bound to the shared ctx stop when the caller cancels it.
func (t *tenantMux) Close() {
	t.mu.Lock()
	defer t.mu.Unlock()
	for id, tn := range t.tenants {
		<-tn.ready
		if tn.cleanup != nil {
			tn.cleanup()
		}
		delete(t.tenants, id)
	}
}

// defaultSessionsDir is ~/.autodj/sessions, the per-browser SQLite home. Falls back
// to a relative dir when the home directory is unavailable.
func defaultSessionsDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "autodj-sessions"
	}
	return filepath.Join(home, ".autodj", "sessions")
}
