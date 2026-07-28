package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"autodj/adapters/driven/memory"
	"autodj/domain/shared"
	"autodj/domain/spotify"
	"autodj/ports/inbound"
)

// fakeCatalog is a scripted SpotifyCatalog recording the token and returning
// canned items so import/search logic is tested without the Web API.
type fakeCatalog struct {
	searchItems   []spotify.CatalogItem
	searchErr     error
	tracksItems   []spotify.CatalogItem
	tracksErr     error
	playlistItems []spotify.CatalogItem
	playlistErr   error

	gotToken    string
	gotQuery    string
	gotKind     string
	gotURIs     []string
	gotPlaylist string
}

func (f *fakeCatalog) Search(_ context.Context, token, query, kind string) ([]spotify.CatalogItem, error) {
	f.gotToken, f.gotQuery, f.gotKind = token, query, kind
	return f.searchItems, f.searchErr
}
func (f *fakeCatalog) Tracks(_ context.Context, token string, uris []string) ([]spotify.CatalogItem, error) {
	f.gotToken, f.gotURIs = token, uris
	return f.tracksItems, f.tracksErr
}
func (f *fakeCatalog) PlaylistTracks(_ context.Context, token, playlistID string) ([]spotify.CatalogItem, error) {
	f.gotToken, f.gotPlaylist = token, playlistID
	return f.playlistItems, f.playlistErr
}

func trackItem(uri, title, artist string, durMs int) spotify.CatalogItem {
	return spotify.CatalogItem{URI: uri, Title: title, Artist: artist, Album: "Album X", DurationMs: durMs, Kind: spotify.KindTrack}
}

func linkedSvc(t *testing.T, oauth *fakeOAuth, cat *fakeCatalog) (*SpotifyService, *memory.TrackRepo) {
	t.Helper()
	accRepo := memory.NewSpotifyAccountRepo()
	_ = accRepo.Save(context.Background(), premiumAccount(t0.Add(time.Hour)))
	tracks := memory.New()
	svc := NewSpotifyService(oauth, accRepo, &movableClock{t: t0}).WithCatalog(cat, tracks)
	return svc, tracks
}

func TestSpotify_Search_unavailableWithoutCatalog(t *testing.T) {
	svc := NewSpotifyService(&fakeOAuth{}, memory.NewSpotifyAccountRepo(), &movableClock{t: t0})
	if _, err := svc.Search(context.Background(), "q", spotify.KindTrack); err != spotify.ErrCatalogUnavailable {
		t.Fatalf("err = %v, want ErrCatalogUnavailable", err)
	}
	if _, err := svc.ImportTracks(context.Background(), []string{"spotify:track:1"}); err != spotify.ErrCatalogUnavailable {
		t.Fatalf("import err = %v, want ErrCatalogUnavailable", err)
	}
	if _, err := svc.ImportPlaylist(context.Background(), "p"); err != spotify.ErrCatalogUnavailable {
		t.Fatalf("playlist err = %v, want ErrCatalogUnavailable", err)
	}
}

func TestSpotify_Search_ok(t *testing.T) {
	cat := &fakeCatalog{searchItems: []spotify.CatalogItem{trackItem("spotify:track:1", "Neon Skyline", "Aurora Lane", 210000)}}
	svc, _ := linkedSvc(t, &fakeOAuth{}, cat)
	items, err := svc.Search(context.Background(), "neon", spotify.KindTrack)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Title != "Neon Skyline" {
		t.Fatalf("items = %+v", items)
	}
	if cat.gotToken != "acc-tok" || cat.gotQuery != "neon" || cat.gotKind != spotify.KindTrack {
		t.Fatalf("catalog got token=%q query=%q kind=%q", cat.gotToken, cat.gotQuery, cat.gotKind)
	}
}

func TestSpotify_Search_noAccount(t *testing.T) {
	cat := &fakeCatalog{}
	svc := NewSpotifyService(&fakeOAuth{}, memory.NewSpotifyAccountRepo(), &movableClock{t: t0}).WithCatalog(cat, memory.New())
	if _, err := svc.Search(context.Background(), "q", spotify.KindTrack); err != spotify.ErrNoAccount {
		t.Fatalf("err = %v, want ErrNoAccount", err)
	}
}

func TestSpotify_Search_catalogError(t *testing.T) {
	cat := &fakeCatalog{searchErr: errors.New("api 500")}
	svc, _ := linkedSvc(t, &fakeOAuth{}, cat)
	if _, err := svc.Search(context.Background(), "q", spotify.KindTrack); err == nil {
		t.Fatal("want catalog error")
	}
}

func TestSpotify_ImportTracks_ok(t *testing.T) {
	cat := &fakeCatalog{tracksItems: []spotify.CatalogItem{
		trackItem("spotify:track:1", "Amber Waves", "Hollow Coves", 180000),
		trackItem("spotify:track:2", "Glass Harbor", "Wintergreen", 200000),
	}}
	svc, tracks := linkedSvc(t, &fakeOAuth{}, cat)
	out, err := svc.ImportTracks(context.Background(), []string{"spotify:track:1", "spotify:track:2"})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 {
		t.Fatalf("len = %d, want 2", len(out))
	}
	// Two-engine invariant: Spotify import is non-mixable, non-PCM.
	for _, tr := range out {
		if tr.Source != shared.SourceSpotify || tr.Mixable || tr.PCMAvailable {
			t.Fatalf("invariant broken: %+v", tr)
		}
		if tr.SpotifyURI == "" || tr.Artist == "" {
			t.Fatalf("metadata missing: %+v", tr)
		}
	}
	stored, _ := tracks.List(context.Background())
	if len(stored) != 2 {
		t.Fatalf("stored = %d, want 2", len(stored))
	}
	if cat.gotURIs[0] != "spotify:track:1" {
		t.Fatalf("uris = %v", cat.gotURIs)
	}
}

func TestSpotify_Import_skipsPlaylistAndZeroDuration(t *testing.T) {
	cat := &fakeCatalog{tracksItems: []spotify.CatalogItem{
		trackItem("spotify:track:1", "Real Track", "Artist", 190000),
		{URI: "spotify:playlist:9", Title: "a playlist", Kind: spotify.KindPlaylist},
		trackItem("spotify:track:2", "No Duration", "Ghost", 0),
		{URI: "", Title: "empty", Kind: spotify.KindTrack, DurationMs: 1000},
	}}
	svc, tracks := linkedSvc(t, &fakeOAuth{}, cat)
	out, err := svc.ImportTracks(context.Background(), []string{"x"})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0].Title != "Real Track" {
		t.Fatalf("out = %+v, want just Real Track", out)
	}
	stored, _ := tracks.List(context.Background())
	if len(stored) != 1 {
		t.Fatalf("stored = %d, want 1", len(stored))
	}
}

func TestSpotify_ImportTracks_dedupeOnReimport(t *testing.T) {
	cat := &fakeCatalog{tracksItems: []spotify.CatalogItem{trackItem("spotify:track:1", "Amber Waves", "Hollow Coves", 180000)}}
	svc, tracks := linkedSvc(t, &fakeOAuth{}, cat)
	_, _ = svc.ImportTracks(context.Background(), []string{"spotify:track:1"})
	_, _ = svc.ImportTracks(context.Background(), []string{"spotify:track:1"})
	stored, _ := tracks.List(context.Background())
	if len(stored) != 1 {
		t.Fatalf("stored = %d, want 1 (re-import upserts same id)", len(stored))
	}
}

func TestSpotify_ImportTracks_catalogError(t *testing.T) {
	cat := &fakeCatalog{tracksErr: errors.New("api 500")}
	svc, _ := linkedSvc(t, &fakeOAuth{}, cat)
	if _, err := svc.ImportTracks(context.Background(), []string{"x"}); err == nil {
		t.Fatal("want catalog error")
	}
}

func TestSpotify_ImportPlaylist_ok(t *testing.T) {
	cat := &fakeCatalog{playlistItems: []spotify.CatalogItem{
		trackItem("spotify:track:3", "Coastlines", "Hollow Coves", 175000),
	}}
	svc, tracks := linkedSvc(t, &fakeOAuth{}, cat)
	out, err := svc.ImportPlaylist(context.Background(), "spotify:playlist:9")
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0].Source != shared.SourceSpotify {
		t.Fatalf("out = %+v", out)
	}
	if cat.gotPlaylist != "spotify:playlist:9" {
		t.Fatalf("playlist = %q", cat.gotPlaylist)
	}
	stored, _ := tracks.List(context.Background())
	if len(stored) != 1 {
		t.Fatalf("stored = %d", len(stored))
	}
}

func TestSpotify_ImportPlaylist_catalogError(t *testing.T) {
	cat := &fakeCatalog{playlistErr: errors.New("api 404")}
	svc, _ := linkedSvc(t, &fakeOAuth{}, cat)
	if _, err := svc.ImportPlaylist(context.Background(), "p"); err == nil {
		t.Fatal("want catalog error")
	}
}

// fakeOAuth is a scripted SpotifyOAuth: it records calls and returns canned
// accounts so the service logic is tested without the network.
type fakeOAuth struct {
	state      string
	stateQueue []string // if set, NewState pops from here (multi-Begin tests)
	verifier   string
	challenge  string
	stateErr   error
	pkceErr    error

	// Probe: o que o Spotify responde ao diagnóstico (perfil + devices)
	probe    spotify.Probe
	probeErr error

	exchanged spotify.Account
	exchErr   error
	gotCode   string
	gotVer    string

	refreshed  spotify.Account
	refreshErr error
	gotRefresh string

	authURL string
	gotScopes []string
}

func (f *fakeOAuth) AuthURL(state, challenge string, scopes []string) string {
	f.gotScopes = scopes
	return f.authURL
}
func (f *fakeOAuth) NewPKCE() (string, string, error) { return f.verifier, f.challenge, f.pkceErr }
func (f *fakeOAuth) NewState() (string, error) {
	if len(f.stateQueue) > 0 {
		s := f.stateQueue[0]
		f.stateQueue = f.stateQueue[1:]
		return s, f.stateErr
	}
	return f.state, f.stateErr
}
func (f *fakeOAuth) Exchange(_ context.Context, code, verifier string) (spotify.Account, error) {
	f.gotCode, f.gotVer = code, verifier
	return f.exchanged, f.exchErr
}
func (f *fakeOAuth) Refresh(_ context.Context, rt string) (spotify.Account, error) {
	f.gotRefresh = rt
	return f.refreshed, f.refreshErr
}

func (f *fakeOAuth) Probe(context.Context, string) (spotify.Probe, error) {
	return f.probe, f.probeErr
}

// O caso REAL que motivou o diagnóstico: o Spotify aceitava connect, registrava o
// device e respondia 204 ao play — sem um único evento de erro — e não emitia som.
// O único sinal honesto era um 403 no perfil, visível só do servidor. Este teste
// trava a tradução desse 403 na instrução certa (allowlist / Development Mode).
func TestDiagnose_403NoPerfilApontaAllowlist(t *testing.T) {
	acc := spotify.Account{AccessToken: "a", RefreshToken: "r", Product: "premium"}
	oauth := &fakeOAuth{refreshed: acc, probe: spotify.Probe{ProfileStatus: 403}}
	svc := diagSvc(t, oauth, acc)

	d, err := svc.Diagnose(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if d.OK {
		t.Fatal("diagnostico nao pode passar com 403 no perfil")
	}
	perfil := stepByKey(d, "perfil")
	if perfil == nil || perfil.OK {
		t.Fatalf("degrau perfil = %+v, queria falha", perfil)
	}
	if !strings.Contains(perfil.Hint, "User Management") {
		t.Fatalf("a dica precisa mandar o operador pra allowlist, veio: %q", perfil.Hint)
	}
	// degraus DEPOIS da causa não podem aparecer: confundem quem lê.
	if stepByKey(d, "device") != nil {
		t.Fatal("nao devia checar device depois de falhar no perfil")
	}
}

// Token que não renova é a outra causa clássica, e pede ação diferente (reconectar).
func TestDiagnose_TokenMortoPedeReconectar(t *testing.T) {
	acc := spotify.Account{AccessToken: "a", RefreshToken: "r"}
	oauth := &fakeOAuth{refreshErr: errors.New("invalid_grant")}
	svc := diagSvc(t, oauth, acc)

	d, _ := svc.Diagnose(context.Background())
	tok := stepByKey(d, "token")
	if tok == nil || tok.OK || !strings.Contains(tok.Hint, "Reconecte") {
		t.Fatalf("degrau token = %+v", tok)
	}
	if stepByKey(d, "perfil") != nil {
		t.Fatal("nao devia consultar o perfil com o token morto")
	}
}

// Caminho feliz: todos os degraus passam e o veredito fica vazio.
func TestDiagnose_TudoCerto(t *testing.T) {
	acc := spotify.Account{AccessToken: "a", RefreshToken: "r", Product: "premium"}
	oauth := &fakeOAuth{
		refreshed: acc,
		probe: spotify.Probe{
			ProfileStatus: 200, Product: "premium",
			DevicesStatus: 200, Devices: []string{"AutoDJ Playout"}, ActiveDevice: "AutoDJ Playout",
		},
	}
	svc := diagSvc(t, oauth, acc)

	d, _ := svc.Diagnose(context.Background())
	if !d.OK || d.Verdict != "" {
		t.Fatalf("queria OK sem veredito, veio ok=%v verdict=%q", d.OK, d.Verdict)
	}
	for _, s := range d.Steps {
		if !s.OK {
			t.Fatalf("degrau %q falhou: %+v", s.Key, s)
		}
	}
	if len(d.Steps) != 5 {
		t.Fatalf("queria 5 degraus, veio %d", len(d.Steps))
	}
}

// Conta sem Premium: o SDK do navegador simplesmente não toca, e o diagnóstico
// precisa dizer isso em vez de deixar o operador procurando em outro lugar.
func TestDiagnose_NoPremium(t *testing.T) {
	acc := spotify.Account{AccessToken: "a", RefreshToken: "r", Product: "free"}
	oauth := &fakeOAuth{refreshed: acc, probe: spotify.Probe{ProfileStatus: 200, Product: "free"}}
	svc := diagSvc(t, oauth, acc)

	d, _ := svc.Diagnose(context.Background())
	prem := stepByKey(d, "premium")
	if prem == nil || prem.OK {
		t.Fatalf("degrau premium = %+v", prem)
	}
	if !strings.Contains(d.Verdict, "Premium") {
		t.Fatalf("veredito = %q", d.Verdict)
	}
}

// diagSvc monta um serviço com a conta dada (o diagnóstico depende do que está
// guardado, não de catálogo).
func diagSvc(t *testing.T, oauth *fakeOAuth, acc spotify.Account) *SpotifyService {
	t.Helper()
	accRepo := memory.NewSpotifyAccountRepo()
	if err := accRepo.Save(context.Background(), acc); err != nil {
		t.Fatal(err)
	}
	return NewSpotifyService(oauth, accRepo, &movableClock{t: t0})
}

func stepByKey(d inbound.SpotifyDiagnosis, key string) *inbound.SpotifyStep {
	for i := range d.Steps {
		if d.Steps[i].Key == key {
			return &d.Steps[i]
		}
	}
	return nil
}

// movableClock is an advanceable clock for refresh-window tests.
type movableClock struct{ t time.Time }

func (c *movableClock) Now() time.Time { return c.t }

var t0 = time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)

func premiumAccount(expiresAt time.Time) spotify.Account {
	return spotify.Account{
		AccessToken:  "acc-tok",
		RefreshToken: "ref-tok",
		ExpiresAt:    expiresAt,
		Scopes:       []string{"streaming"},
		Product:      "premium",
	}
}

func TestSpotify_Begin_storesPendingAndReturnsURL(t *testing.T) {
	oauth := &fakeOAuth{state: "st8", verifier: "vfy", challenge: "chg", authURL: "https://accounts/authorize?x=1"}
	svc := NewSpotifyService(oauth, memory.NewSpotifyAccountRepo(), &movableClock{t: t0})

	url, err := svc.Begin(context.Background())
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if url != oauth.authURL {
		t.Fatalf("url = %q", url)
	}
	if svc.pending["st8"] == nil || svc.pending["st8"].State != "st8" || svc.pending["st8"].Verifier != "vfy" {
		t.Fatalf("pending not stored: %+v", svc.pending)
	}
	if len(oauth.gotScopes) == 0 {
		t.Fatal("scopes not passed to AuthURL")
	}
}

func TestSpotify_Begin_stateError(t *testing.T) {
	oauth := &fakeOAuth{stateErr: errors.New("rng down")}
	svc := NewSpotifyService(oauth, memory.NewSpotifyAccountRepo(), &movableClock{t: t0})
	if _, err := svc.Begin(context.Background()); err == nil {
		t.Fatal("want state error")
	}
}

func TestSpotify_Begin_pkceError(t *testing.T) {
	oauth := &fakeOAuth{state: "s", pkceErr: errors.New("rng down")}
	svc := NewSpotifyService(oauth, memory.NewSpotifyAccountRepo(), &movableClock{t: t0})
	if _, err := svc.Begin(context.Background()); err == nil {
		t.Fatal("want pkce error")
	}
}

func TestSpotify_Complete_ok(t *testing.T) {
	oauth := &fakeOAuth{state: "st8", verifier: "vfy", challenge: "chg", exchanged: premiumAccount(t0.Add(time.Hour))}
	repo := memory.NewSpotifyAccountRepo()
	svc := NewSpotifyService(oauth, repo, &movableClock{t: t0})

	if _, err := svc.Begin(context.Background()); err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := svc.Complete(context.Background(), "st8", "code123"); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if oauth.gotCode != "code123" || oauth.gotVer != "vfy" {
		t.Fatalf("exchange got code=%q ver=%q", oauth.gotCode, oauth.gotVer)
	}
	acc, err := repo.Load(context.Background())
	if err != nil || !acc.Connected() {
		t.Fatalf("account not persisted: %v %+v", err, acc)
	}
	if len(svc.pending) != 0 {
		t.Fatal("pending must clear after complete")
	}
}

func TestSpotify_Complete_noPending(t *testing.T) {
	svc := NewSpotifyService(&fakeOAuth{}, memory.NewSpotifyAccountRepo(), &movableClock{t: t0})
	if err := svc.Complete(context.Background(), "s", "c"); err != spotify.ErrNoAccount {
		t.Fatalf("err = %v, want ErrNoAccount", err)
	}
}

func TestSpotify_Complete_expired(t *testing.T) {
	oauth := &fakeOAuth{state: "st8", verifier: "vfy"}
	clk := &movableClock{t: t0}
	svc := NewSpotifyService(oauth, memory.NewSpotifyAccountRepo(), clk)
	_, _ = svc.Begin(context.Background())
	clk.t = t0.Add(11 * time.Minute) // past the 10m pending TTL
	if err := svc.Complete(context.Background(), "st8", "c"); err != spotify.ErrAuthExpired {
		t.Fatalf("err = %v, want ErrAuthExpired", err)
	}
	if len(svc.pending) != 0 {
		t.Fatal("expired pending must clear")
	}
}

func TestSpotify_Complete_stateMismatch(t *testing.T) {
	oauth := &fakeOAuth{state: "st8", verifier: "vfy"}
	svc := NewSpotifyService(oauth, memory.NewSpotifyAccountRepo(), &movableClock{t: t0})
	_, _ = svc.Begin(context.Background())
	if err := svc.Complete(context.Background(), "WRONG", "c"); err != spotify.ErrStateMismatch {
		t.Fatalf("err = %v, want ErrStateMismatch", err)
	}
}

// TestSpotify_Complete_toleratesSecondBegin mirrors the YouTube regression:
// two Begin() calls no longer invalidate the first authorize page.
func TestSpotify_Complete_toleratesSecondBegin(t *testing.T) {
	oauth := &fakeOAuth{stateQueue: []string{"first", "second"}, verifier: "vfy", exchanged: premiumAccount(t0.Add(time.Hour))}
	repo := memory.NewSpotifyAccountRepo()
	svc := NewSpotifyService(oauth, repo, &movableClock{t: t0})

	if _, err := svc.Begin(context.Background()); err != nil { // pending["first"]
		t.Fatalf("Begin 1: %v", err)
	}
	if _, err := svc.Begin(context.Background()); err != nil { // pending["second"]
		t.Fatalf("Begin 2: %v", err)
	}
	if err := svc.Complete(context.Background(), "first", "code123"); err != nil {
		t.Fatalf("Complete(first) = %v, want nil", err)
	}
	if len(svc.pending) != 0 {
		t.Fatalf("pending must clear after complete, got %d", len(svc.pending))
	}
}

func TestSpotify_Complete_exchangeError(t *testing.T) {
	oauth := &fakeOAuth{state: "st8", verifier: "vfy", exchErr: errors.New("spotify 400")}
	svc := NewSpotifyService(oauth, memory.NewSpotifyAccountRepo(), &movableClock{t: t0})
	_, _ = svc.Begin(context.Background())
	if err := svc.Complete(context.Background(), "st8", "c"); err == nil {
		t.Fatal("want exchange error")
	}
}

func TestSpotify_Status(t *testing.T) {
	repo := memory.NewSpotifyAccountRepo()
	svc := NewSpotifyService(&fakeOAuth{}, repo, &movableClock{t: t0})

	// Disconnected.
	st, err := svc.Status(context.Background())
	if err != nil || st.Connected || st.Premium {
		t.Fatalf("empty status = %+v err %v", st, err)
	}
	// Connected + premium.
	if err := repo.Save(context.Background(), premiumAccount(t0.Add(time.Hour))); err != nil {
		t.Fatalf("save: %v", err)
	}
	st, err = svc.Status(context.Background())
	if err != nil || !st.Connected || !st.Premium {
		t.Fatalf("connected status = %+v err %v", st, err)
	}
}

func TestSpotify_Disconnect(t *testing.T) {
	repo := memory.NewSpotifyAccountRepo()
	_ = repo.Save(context.Background(), premiumAccount(t0.Add(time.Hour)))
	svc := NewSpotifyService(&fakeOAuth{}, repo, &movableClock{t: t0})
	if err := svc.Disconnect(context.Background()); err != nil {
		t.Fatalf("Disconnect: %v", err)
	}
	if _, err := repo.Load(context.Background()); err != spotify.ErrNoAccount {
		t.Fatalf("account still linked: %v", err)
	}
	// Idempotent.
	if err := svc.Disconnect(context.Background()); err != nil {
		t.Fatalf("second Disconnect: %v", err)
	}
}

func TestSpotify_AccessToken_noAccount(t *testing.T) {
	svc := NewSpotifyService(&fakeOAuth{}, memory.NewSpotifyAccountRepo(), &movableClock{t: t0})
	if _, err := svc.AccessToken(context.Background()); err != spotify.ErrNoAccount {
		t.Fatalf("err = %v, want ErrNoAccount", err)
	}
}

func TestSpotify_AccessToken_fresh(t *testing.T) {
	repo := memory.NewSpotifyAccountRepo()
	_ = repo.Save(context.Background(), premiumAccount(t0.Add(time.Hour)))
	oauth := &fakeOAuth{}
	svc := NewSpotifyService(oauth, repo, &movableClock{t: t0})
	tok, err := svc.AccessToken(context.Background())
	if err != nil || tok != "acc-tok" {
		t.Fatalf("tok = %q err %v", tok, err)
	}
	if oauth.gotRefresh != "" {
		t.Fatal("fresh token must not refresh")
	}
}

func TestSpotify_AccessToken_refreshes(t *testing.T) {
	repo := memory.NewSpotifyAccountRepo()
	_ = repo.Save(context.Background(), premiumAccount(t0.Add(30*time.Second))) // inside refresh window
	oauth := &fakeOAuth{refreshed: spotify.Account{AccessToken: "new-tok", RefreshToken: "ref-tok", ExpiresAt: t0.Add(time.Hour), Product: "premium"}}
	svc := NewSpotifyService(oauth, repo, &movableClock{t: t0})
	tok, err := svc.AccessToken(context.Background())
	if err != nil || tok != "new-tok" {
		t.Fatalf("tok = %q err %v", tok, err)
	}
	if oauth.gotRefresh != "ref-tok" {
		t.Fatalf("refresh got %q", oauth.gotRefresh)
	}
	acc, _ := repo.Load(context.Background())
	if acc.AccessToken != "new-tok" {
		t.Fatalf("refreshed token not persisted: %q", acc.AccessToken)
	}
}

func TestSpotify_AccessToken_refreshError(t *testing.T) {
	repo := memory.NewSpotifyAccountRepo()
	_ = repo.Save(context.Background(), premiumAccount(t0.Add(30*time.Second)))
	oauth := &fakeOAuth{refreshErr: errors.New("spotify 401")}
	svc := NewSpotifyService(oauth, repo, &movableClock{t: t0})
	if _, err := svc.AccessToken(context.Background()); err == nil {
		t.Fatal("want refresh error")
	}
}
