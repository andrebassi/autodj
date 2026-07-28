package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"autodj/domain/library"
	"autodj/domain/shared"
	"autodj/domain/youtube"
	"autodj/ports/inbound"
)

// fakeYouTube is a configurable YouTubeService stub.
type fakeYouTube struct {
	authURL      string
	beginErr     error
	completeErr  error
	status       inbound.YouTubeStatus
	statusErr    error
	disconnErr   error
	gotState     string
	gotCode      string
	disconnected bool

	searchItems  []youtube.CatalogItem
	searchErr    error
	importTracks []library.Track
	importErr    error
	gotQuery     string
	gotKind      string
	gotBackend   string
	gotURIs      []string
	gotPlaylist  string
	gotLinks     string
	linksTracks  []library.Track
	linksErr     error

	accessToken string
	tokenErr    error
}

func (f *fakeYouTube) Begin(context.Context) (string, error) { return f.authURL, f.beginErr }
func (f *fakeYouTube) Complete(_ context.Context, state, code string) error {
	f.gotState, f.gotCode = state, code
	return f.completeErr
}
func (f *fakeYouTube) Status(context.Context) (inbound.YouTubeStatus, error) {
	return f.status, f.statusErr
}
func (f *fakeYouTube) Disconnect(context.Context) error {
	f.disconnected = true
	return f.disconnErr
}
func (f *fakeYouTube) AccessToken(context.Context) (string, error) {
	return f.accessToken, f.tokenErr
}
func (f *fakeYouTube) Search(_ context.Context, query, kind, backend string) ([]youtube.CatalogItem, error) {
	f.gotQuery, f.gotKind, f.gotBackend = query, kind, backend
	return f.searchItems, f.searchErr
}
func (f *fakeYouTube) ImportTracks(_ context.Context, uris []string) ([]library.Track, error) {
	f.gotURIs = uris
	return f.importTracks, f.importErr
}
func (f *fakeYouTube) ImportPlaylist(_ context.Context, playlistID string) ([]library.Track, error) {
	f.gotPlaylist = playlistID
	return f.importTracks, f.importErr
}

func (f *fakeYouTube) ImportLinks(_ context.Context, raw string) ([]library.Track, error) {
	f.gotLinks = raw
	return f.linksTracks, f.linksErr
}

func youtubeTrack(t *testing.T, id, uri, title string) library.Track {
	t.Helper()
	tr, err := library.NewTrack(id, shared.SourceYouTube, title, 3*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	tr.YouTubeURI = uri
	tr.Artist = "The Paper Kites"
	return *tr
}

func newYouTubeH(yt *fakeYouTube) http.Handler {
	return New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil, WithYouTube(yt, "/painel"))
}

func TestYouTubeStatusDisconnected(t *testing.T) {
	yt := &fakeYouTube{status: inbound.YouTubeStatus{Connected: false, Premium: false}}
	rec := do(t, newYouTubeH(yt), http.MethodGet, "/api/v1/youtube/status", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	var got inbound.YouTubeStatus
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got.Connected || got.Premium {
		t.Fatalf("body = %+v", got)
	}
}

func TestYouTubeStatusConnected(t *testing.T) {
	yt := &fakeYouTube{status: inbound.YouTubeStatus{Connected: true, Premium: false}}
	rec := do(t, newYouTubeH(yt), http.MethodGet, "/api/v1/youtube/status", "")
	var got inbound.YouTubeStatus
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if !got.Connected {
		t.Fatalf("body = %+v", got)
	}
}

func TestYouTubeStatusError(t *testing.T) {
	yt := &fakeYouTube{statusErr: errBoom}
	rec := do(t, newYouTubeH(yt), http.MethodGet, "/api/v1/youtube/status", "")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("code = %d, want 500", rec.Code)
	}
}

func TestYouTubeConnectOK(t *testing.T) {
	yt := &fakeYouTube{authURL: "https://accounts.google.com/o/oauth2/v2/auth?x=1"}
	rec := do(t, newYouTubeH(yt), http.MethodPost, "/api/v1/youtube/connect", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	var got map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got["authURL"] != yt.authURL {
		t.Fatalf("authURL = %q", got["authURL"])
	}
}

func TestYouTubeConnectError(t *testing.T) {
	yt := &fakeYouTube{beginErr: errBoom}
	rec := do(t, newYouTubeH(yt), http.MethodPost, "/api/v1/youtube/connect", "")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("code = %d, want 500", rec.Code)
	}
}

func TestYouTubeCallbackOK(t *testing.T) {
	yt := &fakeYouTube{}
	rec := do(t, newYouTubeH(yt), http.MethodGet, "/api/v1/youtube/callback?code=abc&state=xyz", "")
	if rec.Code != http.StatusFound {
		t.Fatalf("code = %d, want 302", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/painel" {
		t.Fatalf("Location = %q, want /painel", loc)
	}
	if yt.gotCode != "abc" || yt.gotState != "xyz" {
		t.Fatalf("forwarded code=%q state=%q", yt.gotCode, yt.gotState)
	}
}

func TestYouTubeCallbackStateMismatch(t *testing.T) {
	yt := &fakeYouTube{completeErr: youtube.ErrStateMismatch}
	rec := do(t, newYouTubeH(yt), http.MethodGet, "/api/v1/youtube/callback?code=abc&state=bad", "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

func TestYouTubeCallbackExpired(t *testing.T) {
	yt := &fakeYouTube{completeErr: youtube.ErrAuthExpired}
	rec := do(t, newYouTubeH(yt), http.MethodGet, "/api/v1/youtube/callback?code=abc&state=xyz", "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

func TestYouTubeCallbackNoAccount(t *testing.T) {
	yt := &fakeYouTube{completeErr: youtube.ErrNoAccount}
	rec := do(t, newYouTubeH(yt), http.MethodGet, "/api/v1/youtube/callback?code=abc&state=xyz", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("code = %d, want 404", rec.Code)
	}
}

func TestYouTubeDisconnectOK(t *testing.T) {
	yt := &fakeYouTube{}
	rec := do(t, newYouTubeH(yt), http.MethodPost, "/api/v1/youtube/disconnect", "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("code = %d, want 204", rec.Code)
	}
	if !yt.disconnected {
		t.Fatal("Disconnect not called")
	}
}

func TestYouTubeDisconnectError(t *testing.T) {
	yt := &fakeYouTube{disconnErr: errBoom}
	rec := do(t, newYouTubeH(yt), http.MethodPost, "/api/v1/youtube/disconnect", "")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("code = %d, want 500", rec.Code)
	}
}

func TestYouTubeDefaultReturnPath(t *testing.T) {
	yt := &fakeYouTube{}
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil, WithYouTube(yt, ""))
	rec := do(t, h, http.MethodGet, "/api/v1/youtube/callback?code=a&state=b", "")
	if rec.Header().Get("Location") != "/" {
		t.Fatalf("Location = %q, want /", rec.Header().Get("Location"))
	}
}

func TestYouTubeRoutesAbsentWithoutOption(t *testing.T) {
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil)
	if rec := do(t, h, http.MethodGet, "/api/v1/youtube/status", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("GET should 404 without WithYouTube, got %d", rec.Code)
	}
}

func TestYouTubeSearchOK(t *testing.T) {
	yt := &fakeYouTube{searchItems: []youtube.CatalogItem{
		{URI: "vid1", Title: "Neon Skyline", Artist: "The Paper Kites", DurationMs: 210000, Kind: youtube.KindVideo},
	}}
	rec := do(t, newYouTubeH(yt), http.MethodGet, "/api/v1/youtube/search?q=neon&type=video", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	var got []youtube.CatalogItem
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if len(got) != 1 || got[0].Title != "Neon Skyline" {
		t.Fatalf("body = %+v", got)
	}
	if yt.gotQuery != "neon" || yt.gotKind != "video" {
		t.Fatalf("forwarded query=%q kind=%q", yt.gotQuery, yt.gotKind)
	}
}

func TestYouTubeSearchDefaultKind(t *testing.T) {
	yt := &fakeYouTube{}
	do(t, newYouTubeH(yt), http.MethodGet, "/api/v1/youtube/search?q=x", "")
	if yt.gotKind != youtube.KindVideo {
		t.Fatalf("kind = %q, want default video", yt.gotKind)
	}
}

// O seletor de backend é repassado cru — quem normaliza é o domínio, não o HTTP.
func TestYouTubeSearchForwardsBackend(t *testing.T) {
	yt := &fakeYouTube{}
	do(t, newYouTubeH(yt), http.MethodGet, "/api/v1/youtube/search?q=x&backend=api", "")
	if yt.gotBackend != youtube.BackendAPI {
		t.Fatalf("backend = %q, want %q", yt.gotBackend, youtube.BackendAPI)
	}
}

// Cliente antigo não manda ?backend — chega vazio e o serviço decide (free).
func TestYouTubeSearchBackendOmitted(t *testing.T) {
	yt := &fakeYouTube{}
	do(t, newYouTubeH(yt), http.MethodGet, "/api/v1/youtube/search?q=x", "")
	if yt.gotBackend != "" {
		t.Fatalf("backend = %q, want empty", yt.gotBackend)
	}
}

func TestYouTubeSearchEmptyIsArray(t *testing.T) {
	yt := &fakeYouTube{searchItems: nil}
	rec := do(t, newYouTubeH(yt), http.MethodGet, "/api/v1/youtube/search?q=x", "")
	if body := rec.Body.String(); body != "[]\n" && body != "[]" {
		t.Fatalf("body = %q, want []", body)
	}
}

func TestYouTubeSearchNoAccount(t *testing.T) {
	yt := &fakeYouTube{searchErr: youtube.ErrNoAccount}
	rec := do(t, newYouTubeH(yt), http.MethodGet, "/api/v1/youtube/search?q=x", "")
	if rec.Code != http.StatusConflict {
		t.Fatalf("code = %d, want 409", rec.Code)
	}
}

func TestYouTubeSearchUnavailable(t *testing.T) {
	yt := &fakeYouTube{searchErr: youtube.ErrCatalogUnavailable}
	rec := do(t, newYouTubeH(yt), http.MethodGet, "/api/v1/youtube/search?q=x", "")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("code = %d, want 503", rec.Code)
	}
}

func TestYouTubeSearchInternalError(t *testing.T) {
	yt := &fakeYouTube{searchErr: errBoom}
	rec := do(t, newYouTubeH(yt), http.MethodGet, "/api/v1/youtube/search?q=x", "")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("code = %d, want 500", rec.Code)
	}
}

func TestYouTubeImportTracksOK(t *testing.T) {
	yt := &fakeYouTube{importTracks: []library.Track{youtubeTrack(t, "id1", "vid1", "Neon Skyline")}}
	rec := do(t, newYouTubeH(yt), http.MethodPost, "/api/v1/youtube/import", `{"uris":["vid1"]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	var got []trackDTO
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if len(got) != 1 || got[0].Source != string(shared.SourceYouTube) || got[0].Mixable {
		t.Fatalf("dto = %+v (want non-mixable youtube)", got)
	}
	if len(yt.gotURIs) != 1 || yt.gotURIs[0] != "vid1" {
		t.Fatalf("uris = %v", yt.gotURIs)
	}
}

func TestYouTubeImportPlaylistOK(t *testing.T) {
	yt := &fakeYouTube{importTracks: []library.Track{youtubeTrack(t, "id2", "vid2", "Amber Waves")}}
	rec := do(t, newYouTubeH(yt), http.MethodPost, "/api/v1/youtube/import", `{"playlistID":"pl9"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	if yt.gotPlaylist != "pl9" {
		t.Fatalf("playlist = %q", yt.gotPlaylist)
	}
}

func TestYouTubeImportBadBody(t *testing.T) {
	rec := do(t, newYouTubeH(&fakeYouTube{}), http.MethodPost, "/api/v1/youtube/import", `not json`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

func TestYouTubeImportEmptyRequest(t *testing.T) {
	rec := do(t, newYouTubeH(&fakeYouTube{}), http.MethodPost, "/api/v1/youtube/import", `{}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400 (uris or playlistID required)", rec.Code)
	}
}

func TestYouTubeImportNoAccount(t *testing.T) {
	yt := &fakeYouTube{importErr: youtube.ErrNoAccount}
	rec := do(t, newYouTubeH(yt), http.MethodPost, "/api/v1/youtube/import", `{"uris":["x"]}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("code = %d, want 409", rec.Code)
	}
}

func TestYouTubeImportUnavailable(t *testing.T) {
	yt := &fakeYouTube{importErr: youtube.ErrCatalogUnavailable}
	rec := do(t, newYouTubeH(yt), http.MethodPost, "/api/v1/youtube/import", `{"uris":["x"]}`)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("code = %d, want 503", rec.Code)
	}
}

func TestYouTubeTokenOK(t *testing.T) {
	yt := &fakeYouTube{accessToken: "tok-abc"}
	rec := do(t, newYouTubeH(yt), http.MethodGet, "/api/v1/youtube/token", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	var got map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got["accessToken"] != "tok-abc" {
		t.Fatalf("accessToken = %q, want tok-abc", got["accessToken"])
	}
}

func TestYouTubeTokenNoAccount(t *testing.T) {
	yt := &fakeYouTube{tokenErr: youtube.ErrNoAccount}
	rec := do(t, newYouTubeH(yt), http.MethodGet, "/api/v1/youtube/token", "")
	if rec.Code != http.StatusConflict {
		t.Fatalf("code = %d, want 409", rec.Code)
	}
}

func TestYouTubeTokenError(t *testing.T) {
	yt := &fakeYouTube{tokenErr: errBoom}
	rec := do(t, newYouTubeH(yt), http.MethodGet, "/api/v1/youtube/token", "")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("code = %d, want 500", rec.Code)
	}
}

// --- POST /youtube/import-links: colar links, sem login (0.2.17) ---

func TestYouTubeImportLinksOK(t *testing.T) {
	yt := &fakeYouTube{linksTracks: []library.Track{
		youtubeTrack(t, "id1", "vid1", "Neon Skyline"),
		youtubeTrack(t, "id2", "vid2", "Amber Waves"),
	}}
	body := `{"text":"https://youtu.be/dQw4w9WgXcQ; https://www.youtube.com/watch?v=9bZkp7q19f0"}`
	rec := do(t, newYouTubeH(yt), http.MethodPost, "/api/v1/youtube/import-links", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	var got []trackDTO
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if len(got) != 2 || got[0].Source != string(shared.SourceYouTube) || got[0].Mixable {
		t.Fatalf("dto = %+v (want 2 faixas youtube não-mixáveis)", got)
	}
	if !strings.Contains(yt.gotLinks, "dQw4w9WgXcQ") {
		t.Fatalf("texto colado não chegou ao serviço: %q", yt.gotLinks)
	}
}

func TestYouTubeImportLinksBadBody(t *testing.T) {
	rec := do(t, newYouTubeH(&fakeYouTube{}), http.MethodPost, "/api/v1/youtube/import-links", "{")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

func TestYouTubeImportLinksNoLinks(t *testing.T) {
	yt := &fakeYouTube{linksErr: youtube.ErrNoLinks}
	rec := do(t, newYouTubeH(yt), http.MethodPost, "/api/v1/youtube/import-links", `{"text":"nada"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

func TestYouTubeImportLinksQuota(t *testing.T) {
	yt := &fakeYouTube{linksErr: youtube.ErrQuotaExceeded}
	rec := do(t, newYouTubeH(yt), http.MethodPost, "/api/v1/youtube/import-links", `{"text":"https://youtu.be/dQw4w9WgXcQ"}`)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("code = %d, want 429", rec.Code)
	}
}

// servidor só com API key (sem cliente OAuth): conectar responde 503, mas o
// import por links segue funcionando — a aba não pode virar beco sem saída.
func TestYouTubeConnectOAuthUnavailable(t *testing.T) {
	yt := &fakeYouTube{beginErr: youtube.ErrOAuthUnavailable}
	rec := do(t, newYouTubeH(yt), http.MethodPost, "/api/v1/youtube/connect", "")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("code = %d, want 503", rec.Code)
	}
}

func TestYouTubeCallbackOAuthUnavailable(t *testing.T) {
	yt := &fakeYouTube{completeErr: youtube.ErrOAuthUnavailable}
	rec := do(t, newYouTubeH(yt), http.MethodGet, "/api/v1/youtube/callback?code=a&state=b", "")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("code = %d, want 503", rec.Code)
	}
}
