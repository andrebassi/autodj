package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"autodj/domain/library"
	"autodj/domain/shared"
	"autodj/domain/spotify"
	"autodj/ports/inbound"
)

// fakeSpotify is a configurable SpotifyService stub.
type fakeSpotify struct {
	authURL     string
	beginErr    error
	completeErr error
	status      inbound.SpotifyStatus
	statusErr   error
	disconnErr  error
	gotState    string
	gotCode     string
	disconnected bool

	// Revalidate: força renovação do token quando o playout trava (v0.2.24)
	revalidated bool
	revalStatus inbound.SpotifyStatus
	revalErr    error

	// Diagnose: cadeia de verificação (v0.2.26)
	diagnosis inbound.SpotifyDiagnosis
	diagErr   error

	searchItems  []spotify.CatalogItem
	searchErr    error
	importTracks []library.Track
	importErr    error
	gotQuery     string
	gotKind      string
	gotURIs      []string
	gotPlaylist  string

	accessToken string
	tokenErr    error
}

func (f *fakeSpotify) Begin(context.Context) (string, error) { return f.authURL, f.beginErr }
func (f *fakeSpotify) Complete(_ context.Context, state, code string) error {
	f.gotState, f.gotCode = state, code
	return f.completeErr
}
func (f *fakeSpotify) Status(context.Context) (inbound.SpotifyStatus, error) {
	return f.status, f.statusErr
}
func (f *fakeSpotify) Diagnose(context.Context) (inbound.SpotifyDiagnosis, error) {
	return f.diagnosis, f.diagErr
}
func (f *fakeSpotify) Revalidate(context.Context) (inbound.SpotifyStatus, error) {
	f.revalidated = true
	if f.revalErr != nil {
		return inbound.SpotifyStatus{}, f.revalErr
	}
	return f.revalStatus, nil
}
func (f *fakeSpotify) Disconnect(context.Context) error {
	f.disconnected = true
	return f.disconnErr
}
func (f *fakeSpotify) AccessToken(context.Context) (string, error) {
	return f.accessToken, f.tokenErr
}
func (f *fakeSpotify) Search(_ context.Context, query, kind string) ([]spotify.CatalogItem, error) {
	f.gotQuery, f.gotKind = query, kind
	return f.searchItems, f.searchErr
}
func (f *fakeSpotify) ImportTracks(_ context.Context, uris []string) ([]library.Track, error) {
	f.gotURIs = uris
	return f.importTracks, f.importErr
}
func (f *fakeSpotify) ImportPlaylist(_ context.Context, playlistID string) ([]library.Track, error) {
	f.gotPlaylist = playlistID
	return f.importTracks, f.importErr
}

func spotifyTrack(t *testing.T, id, uri, title string) library.Track {
	t.Helper()
	tr, err := library.NewTrack(id, shared.SourceSpotify, title, 3*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	tr.SpotifyURI = uri
	tr.Artist = "Aurora Lane"
	return *tr
}

func newSpotifyH(sp *fakeSpotify) http.Handler {
	return New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil, WithSpotify(sp, "/painel"))
}

func TestSpotifyStatusDisconnected(t *testing.T) {
	sp := &fakeSpotify{status: inbound.SpotifyStatus{Connected: false, Premium: false}}
	rec := do(t, newSpotifyH(sp), http.MethodGet, "/api/v1/spotify/status", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	var got inbound.SpotifyStatus
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got.Connected || got.Premium {
		t.Fatalf("body = %+v", got)
	}
}

// Revalidate é chamado quando o playout trava (device aceita comandos e não toca).
// Sucesso devolve o estado REAL da conta — é o que distingue "sem Premium" de
// "token recusado", em vez de o operador ficar adivinhando no silêncio.
func TestSpotifyRevalidateOK(t *testing.T) {
	sp := &fakeSpotify{revalStatus: inbound.SpotifyStatus{Connected: true, Premium: false}}
	rec := do(t, newSpotifyH(sp), http.MethodPost, "/api/v1/spotify/revalidate", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	if !sp.revalidated {
		t.Fatal("Revalidate não foi chamado no serviço")
	}
	var got inbound.SpotifyStatus
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if !got.Connected || got.Premium {
		t.Fatalf("body = %+v, want conectado sem premium", got)
	}
}

// Renovação falhou = token morto. 409 (não 500): é condição de conta, não erro de
// servidor, e a UI traduz em "reconecte a conta".
func TestSpotifyRevalidateTokenMorto(t *testing.T) {
	sp := &fakeSpotify{revalErr: errors.New("spotify: refresh recusado")}
	rec := do(t, newSpotifyH(sp), http.MethodPost, "/api/v1/spotify/revalidate", "")
	if rec.Code != http.StatusConflict {
		t.Fatalf("code = %d, want 409", rec.Code)
	}
}

func TestSpotifyStatusConnectedPremium(t *testing.T) {
	sp := &fakeSpotify{status: inbound.SpotifyStatus{Connected: true, Premium: true}}
	rec := do(t, newSpotifyH(sp), http.MethodGet, "/api/v1/spotify/status", "")
	var got inbound.SpotifyStatus
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if !got.Connected || !got.Premium {
		t.Fatalf("body = %+v", got)
	}
}

func TestSpotifyStatusError(t *testing.T) {
	sp := &fakeSpotify{statusErr: errBoom}
	rec := do(t, newSpotifyH(sp), http.MethodGet, "/api/v1/spotify/status", "")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("code = %d, want 500", rec.Code)
	}
}

func TestSpotifyConnectOK(t *testing.T) {
	sp := &fakeSpotify{authURL: "https://accounts.spotify.com/authorize?x=1"}
	rec := do(t, newSpotifyH(sp), http.MethodPost, "/api/v1/spotify/connect", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	var got map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got["authURL"] != sp.authURL {
		t.Fatalf("authURL = %q", got["authURL"])
	}
}

func TestSpotifyConnectError(t *testing.T) {
	sp := &fakeSpotify{beginErr: errBoom}
	rec := do(t, newSpotifyH(sp), http.MethodPost, "/api/v1/spotify/connect", "")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("code = %d, want 500", rec.Code)
	}
}

func TestSpotifyCallbackOK(t *testing.T) {
	sp := &fakeSpotify{}
	rec := do(t, newSpotifyH(sp), http.MethodGet, "/api/v1/spotify/callback?code=abc&state=xyz", "")
	if rec.Code != http.StatusFound {
		t.Fatalf("code = %d, want 302", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/painel" {
		t.Fatalf("Location = %q, want /painel", loc)
	}
	if sp.gotCode != "abc" || sp.gotState != "xyz" {
		t.Fatalf("forwarded code=%q state=%q", sp.gotCode, sp.gotState)
	}
}

func TestSpotifyCallbackStateMismatch(t *testing.T) {
	sp := &fakeSpotify{completeErr: spotify.ErrStateMismatch}
	rec := do(t, newSpotifyH(sp), http.MethodGet, "/api/v1/spotify/callback?code=abc&state=bad", "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

func TestSpotifyCallbackExpired(t *testing.T) {
	sp := &fakeSpotify{completeErr: spotify.ErrAuthExpired}
	rec := do(t, newSpotifyH(sp), http.MethodGet, "/api/v1/spotify/callback?code=abc&state=xyz", "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

func TestSpotifyCallbackNoAccount(t *testing.T) {
	sp := &fakeSpotify{completeErr: spotify.ErrNoAccount}
	rec := do(t, newSpotifyH(sp), http.MethodGet, "/api/v1/spotify/callback?code=abc&state=xyz", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("code = %d, want 404", rec.Code)
	}
}

func TestSpotifyDisconnectOK(t *testing.T) {
	sp := &fakeSpotify{}
	rec := do(t, newSpotifyH(sp), http.MethodPost, "/api/v1/spotify/disconnect", "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("code = %d, want 204", rec.Code)
	}
	if !sp.disconnected {
		t.Fatal("Disconnect not called")
	}
}

func TestSpotifyDisconnectError(t *testing.T) {
	sp := &fakeSpotify{disconnErr: errBoom}
	rec := do(t, newSpotifyH(sp), http.MethodPost, "/api/v1/spotify/disconnect", "")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("code = %d, want 500", rec.Code)
	}
}

func TestSpotifyDefaultReturnPath(t *testing.T) {
	// Empty returnPath defaults to "/".
	sp := &fakeSpotify{}
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil, WithSpotify(sp, ""))
	rec := do(t, h, http.MethodGet, "/api/v1/spotify/callback?code=a&state=b", "")
	if rec.Header().Get("Location") != "/" {
		t.Fatalf("Location = %q, want /", rec.Header().Get("Location"))
	}
}

func TestSpotifyRoutesAbsentWithoutOption(t *testing.T) {
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil)
	if rec := do(t, h, http.MethodGet, "/api/v1/spotify/status", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("GET should 404 without WithSpotify, got %d", rec.Code)
	}
}

func TestSpotifySearchOK(t *testing.T) {
	sp := &fakeSpotify{searchItems: []spotify.CatalogItem{
		{URI: "spotify:track:1", Title: "Neon Skyline", Artist: "Aurora Lane", DurationMs: 210000, Kind: spotify.KindTrack},
	}}
	rec := do(t, newSpotifyH(sp), http.MethodGet, "/api/v1/spotify/search?q=neon&type=track", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	var got []spotify.CatalogItem
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if len(got) != 1 || got[0].Title != "Neon Skyline" {
		t.Fatalf("body = %+v", got)
	}
	if sp.gotQuery != "neon" || sp.gotKind != "track" {
		t.Fatalf("forwarded query=%q kind=%q", sp.gotQuery, sp.gotKind)
	}
}

func TestSpotifySearchDefaultKind(t *testing.T) {
	sp := &fakeSpotify{}
	do(t, newSpotifyH(sp), http.MethodGet, "/api/v1/spotify/search?q=x", "")
	if sp.gotKind != spotify.KindTrack {
		t.Fatalf("kind = %q, want default track", sp.gotKind)
	}
}

func TestSpotifySearchEmptyIsArray(t *testing.T) {
	sp := &fakeSpotify{searchItems: nil}
	rec := do(t, newSpotifyH(sp), http.MethodGet, "/api/v1/spotify/search?q=x", "")
	if body := rec.Body.String(); body != "[]\n" && body != "[]" {
		t.Fatalf("body = %q, want []", body)
	}
}

func TestSpotifySearchNoAccount(t *testing.T) {
	sp := &fakeSpotify{searchErr: spotify.ErrNoAccount}
	rec := do(t, newSpotifyH(sp), http.MethodGet, "/api/v1/spotify/search?q=x", "")
	if rec.Code != http.StatusConflict {
		t.Fatalf("code = %d, want 409", rec.Code)
	}
}

func TestSpotifySearchUnavailable(t *testing.T) {
	sp := &fakeSpotify{searchErr: spotify.ErrCatalogUnavailable}
	rec := do(t, newSpotifyH(sp), http.MethodGet, "/api/v1/spotify/search?q=x", "")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("code = %d, want 503", rec.Code)
	}
}

func TestSpotifySearchInternalError(t *testing.T) {
	sp := &fakeSpotify{searchErr: errBoom}
	rec := do(t, newSpotifyH(sp), http.MethodGet, "/api/v1/spotify/search?q=x", "")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("code = %d, want 500", rec.Code)
	}
}

func TestSpotifyImportTracksOK(t *testing.T) {
	sp := &fakeSpotify{importTracks: []library.Track{spotifyTrack(t, "id1", "spotify:track:1", "Neon Skyline")}}
	rec := do(t, newSpotifyH(sp), http.MethodPost, "/api/v1/spotify/import", `{"uris":["spotify:track:1"]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	var got []trackDTO
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if len(got) != 1 || got[0].Source != string(shared.SourceSpotify) || got[0].Mixable {
		t.Fatalf("dto = %+v (want non-mixable spotify)", got)
	}
	if len(sp.gotURIs) != 1 || sp.gotURIs[0] != "spotify:track:1" {
		t.Fatalf("uris = %v", sp.gotURIs)
	}
}

func TestSpotifyImportPlaylistOK(t *testing.T) {
	sp := &fakeSpotify{importTracks: []library.Track{spotifyTrack(t, "id2", "spotify:track:2", "Amber Waves")}}
	rec := do(t, newSpotifyH(sp), http.MethodPost, "/api/v1/spotify/import", `{"playlistID":"spotify:playlist:9"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	if sp.gotPlaylist != "spotify:playlist:9" {
		t.Fatalf("playlist = %q", sp.gotPlaylist)
	}
}

func TestSpotifyImportBadBody(t *testing.T) {
	rec := do(t, newSpotifyH(&fakeSpotify{}), http.MethodPost, "/api/v1/spotify/import", `not json`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

func TestSpotifyImportEmptyRequest(t *testing.T) {
	rec := do(t, newSpotifyH(&fakeSpotify{}), http.MethodPost, "/api/v1/spotify/import", `{}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400 (uris or playlistID required)", rec.Code)
	}
}

func TestSpotifyImportNoAccount(t *testing.T) {
	sp := &fakeSpotify{importErr: spotify.ErrNoAccount}
	rec := do(t, newSpotifyH(sp), http.MethodPost, "/api/v1/spotify/import", `{"uris":["x"]}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("code = %d, want 409", rec.Code)
	}
}

func TestSpotifyImportUnavailable(t *testing.T) {
	sp := &fakeSpotify{importErr: spotify.ErrCatalogUnavailable}
	rec := do(t, newSpotifyH(sp), http.MethodPost, "/api/v1/spotify/import", `{"uris":["x"]}`)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("code = %d, want 503", rec.Code)
	}
}

func TestSpotifyTokenOK(t *testing.T) {
	sp := &fakeSpotify{accessToken: "tok-abc"}
	rec := do(t, newSpotifyH(sp), http.MethodGet, "/api/v1/spotify/token", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	var got map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got["accessToken"] != "tok-abc" {
		t.Fatalf("accessToken = %q, want tok-abc", got["accessToken"])
	}
}

func TestSpotifyTokenNoAccount(t *testing.T) {
	sp := &fakeSpotify{tokenErr: spotify.ErrNoAccount}
	rec := do(t, newSpotifyH(sp), http.MethodGet, "/api/v1/spotify/token", "")
	if rec.Code != http.StatusConflict {
		t.Fatalf("code = %d, want 409", rec.Code)
	}
}

func TestSpotifyTokenError(t *testing.T) {
	sp := &fakeSpotify{tokenErr: errBoom}
	rec := do(t, newSpotifyH(sp), http.MethodGet, "/api/v1/spotify/token", "")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("code = %d, want 500", rec.Code)
	}
}
