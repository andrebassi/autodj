package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"autodj/adapters/driven/memory"
	"autodj/domain/shared"
	"autodj/domain/youtube"
)

// fakeYTCatalog is a scripted YouTubeCatalog recording the token and returning
// canned items so import/search logic is tested without the Data API.
type fakeYTCatalog struct {
	searchItems   []youtube.CatalogItem
	searchErr     error
	videosItems   []youtube.CatalogItem
	videosErr     error
	playlistItems []youtube.CatalogItem
	playlistErr   error

	gotToken    string
	gotQuery    string
	gotKind     string
	gotIDs      []string
	gotPlaylist string
}

func (f *fakeYTCatalog) Search(_ context.Context, token, query, kind string) ([]youtube.CatalogItem, error) {
	f.gotToken, f.gotQuery, f.gotKind = token, query, kind
	return f.searchItems, f.searchErr
}
func (f *fakeYTCatalog) Videos(_ context.Context, token string, ids []string) ([]youtube.CatalogItem, error) {
	f.gotToken, f.gotIDs = token, ids
	return f.videosItems, f.videosErr
}
func (f *fakeYTCatalog) PlaylistItems(_ context.Context, token, playlistID string) ([]youtube.CatalogItem, error) {
	f.gotToken, f.gotPlaylist = token, playlistID
	return f.playlistItems, f.playlistErr
}

func ytVideoItem(uri, title, artist string, durMs int) youtube.CatalogItem {
	return youtube.CatalogItem{URI: uri, Title: title, Artist: artist, ArtworkURL: "http://img/m.jpg", DurationMs: durMs, Kind: youtube.KindVideo}
}

// connectedYTAccount is a linked, best-effort-non-premium YouTube account.
func connectedYTAccount(expiresAt time.Time) youtube.Account {
	return youtube.Account{
		AccessToken:  "acc-tok",
		RefreshToken: "ref-tok",
		ExpiresAt:    expiresAt,
		Scopes:       []string{"https://www.googleapis.com/auth/youtube.readonly", "openid"},
		ChannelTitle: "DJ Nova",
	}
}

func linkedYTSvc(t *testing.T, oauth *fakeYTOAuth, cat *fakeYTCatalog) (*YouTubeService, *memory.TrackRepo) {
	t.Helper()
	repo := memory.NewYouTubeAccountRepo()
	_ = repo.Save(context.Background(), connectedYTAccount(t0.Add(time.Hour)))
	tracks := memory.New()
	svc := NewYouTubeService(oauth, repo, &movableClock{t: t0}).WithCatalog(cat, tracks)
	return svc, tracks
}

func TestYouTube_Search_unavailableWithoutCatalog(t *testing.T) {
	svc := NewYouTubeService(&fakeYTOAuth{}, memory.NewYouTubeAccountRepo(), &movableClock{t: t0})
	if _, err := svc.Search(context.Background(), "q", youtube.KindVideo, youtube.BackendAPI); err != youtube.ErrCatalogUnavailable {
		t.Fatalf("err = %v, want ErrCatalogUnavailable", err)
	}
	if _, err := svc.ImportTracks(context.Background(), []string{"vid1"}); err != youtube.ErrCatalogUnavailable {
		t.Fatalf("import err = %v, want ErrCatalogUnavailable", err)
	}
	if _, err := svc.ImportPlaylist(context.Background(), "pl9"); err != youtube.ErrCatalogUnavailable {
		t.Fatalf("playlist err = %v, want ErrCatalogUnavailable", err)
	}
}

func TestYouTube_Search_ok(t *testing.T) {
	cat := &fakeYTCatalog{searchItems: []youtube.CatalogItem{ytVideoItem("vid1", "Neon Skyline", "The Paper Kites", 210000)}}
	svc, _ := linkedYTSvc(t, &fakeYTOAuth{}, cat)
	items, err := svc.Search(context.Background(), "neon", youtube.KindVideo, youtube.BackendAPI)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Title != "Neon Skyline" {
		t.Fatalf("items = %+v", items)
	}
	if cat.gotToken != "acc-tok" || cat.gotQuery != "neon" || cat.gotKind != youtube.KindVideo {
		t.Fatalf("catalog got token=%q query=%q kind=%q", cat.gotToken, cat.gotQuery, cat.gotKind)
	}
}

func TestYouTube_Search_noAccount(t *testing.T) {
	cat := &fakeYTCatalog{}
	svc := NewYouTubeService(&fakeYTOAuth{}, memory.NewYouTubeAccountRepo(), &movableClock{t: t0}).WithCatalog(cat, memory.New())
	if _, err := svc.Search(context.Background(), "q", youtube.KindVideo, youtube.BackendAPI); err != youtube.ErrNoAccount {
		t.Fatalf("err = %v, want ErrNoAccount", err)
	}
}

func TestYouTube_Search_catalogError(t *testing.T) {
	cat := &fakeYTCatalog{searchErr: errors.New("api 500")}
	svc, _ := linkedYTSvc(t, &fakeYTOAuth{}, cat)
	if _, err := svc.Search(context.Background(), "q", youtube.KindVideo, youtube.BackendAPI); err == nil {
		t.Fatal("want catalog error")
	}
}

func TestYouTube_ImportTracks_ok(t *testing.T) {
	cat := &fakeYTCatalog{videosItems: []youtube.CatalogItem{
		ytVideoItem("vid1", "Amber Waves", "Hollow Coves", 180000),
		ytVideoItem("vid2", "Glass Harbor", "Wintergreen", 200000),
	}}
	svc, tracks := linkedYTSvc(t, &fakeYTOAuth{}, cat)
	out, err := svc.ImportTracks(context.Background(), []string{"vid1", "vid2"})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 {
		t.Fatalf("len = %d, want 2", len(out))
	}
	// Two-engine invariant: YouTube import is non-mixable, non-PCM.
	for _, tr := range out {
		if tr.Source != shared.SourceYouTube || tr.Mixable || tr.PCMAvailable {
			t.Fatalf("invariant broken: %+v", tr)
		}
		if tr.YouTubeURI == "" || tr.Artist == "" {
			t.Fatalf("metadata missing: %+v", tr)
		}
	}
	stored, _ := tracks.List(context.Background())
	if len(stored) != 2 {
		t.Fatalf("stored = %d, want 2", len(stored))
	}
	if cat.gotIDs[0] != "vid1" {
		t.Fatalf("ids = %v", cat.gotIDs)
	}
}

func TestYouTube_Import_skipsPlaylistAndZeroDuration(t *testing.T) {
	cat := &fakeYTCatalog{videosItems: []youtube.CatalogItem{
		ytVideoItem("vid1", "Real Track", "Artist", 190000),
		{URI: "pl9", Title: "a playlist", Kind: youtube.KindPlaylist},
		ytVideoItem("vid2", "No Duration", "Ghost", 0),
		{URI: "", Title: "empty", Kind: youtube.KindVideo, DurationMs: 1000},
	}}
	svc, tracks := linkedYTSvc(t, &fakeYTOAuth{}, cat)
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

func TestYouTube_ImportTracks_dedupeOnReimport(t *testing.T) {
	cat := &fakeYTCatalog{videosItems: []youtube.CatalogItem{ytVideoItem("vid1", "Amber Waves", "Hollow Coves", 180000)}}
	svc, tracks := linkedYTSvc(t, &fakeYTOAuth{}, cat)
	_, _ = svc.ImportTracks(context.Background(), []string{"vid1"})
	_, _ = svc.ImportTracks(context.Background(), []string{"vid1"})
	stored, _ := tracks.List(context.Background())
	if len(stored) != 1 {
		t.Fatalf("stored = %d, want 1 (re-import upserts same id)", len(stored))
	}
}

func TestYouTube_ImportTracks_catalogError(t *testing.T) {
	cat := &fakeYTCatalog{videosErr: errors.New("api 500")}
	svc, _ := linkedYTSvc(t, &fakeYTOAuth{}, cat)
	if _, err := svc.ImportTracks(context.Background(), []string{"x"}); err == nil {
		t.Fatal("want catalog error")
	}
}

func TestYouTube_ImportPlaylist_ok(t *testing.T) {
	cat := &fakeYTCatalog{playlistItems: []youtube.CatalogItem{
		ytVideoItem("vid3", "Coastlines", "Hollow Coves", 175000),
	}}
	svc, tracks := linkedYTSvc(t, &fakeYTOAuth{}, cat)
	out, err := svc.ImportPlaylist(context.Background(), "pl9")
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0].Source != shared.SourceYouTube {
		t.Fatalf("out = %+v", out)
	}
	if cat.gotPlaylist != "pl9" {
		t.Fatalf("playlist = %q", cat.gotPlaylist)
	}
	stored, _ := tracks.List(context.Background())
	if len(stored) != 1 {
		t.Fatalf("stored = %d", len(stored))
	}
}

func TestYouTube_ImportPlaylist_catalogError(t *testing.T) {
	cat := &fakeYTCatalog{playlistErr: errors.New("api 404")}
	svc, _ := linkedYTSvc(t, &fakeYTOAuth{}, cat)
	if _, err := svc.ImportPlaylist(context.Background(), "p"); err == nil {
		t.Fatal("want catalog error")
	}
}

// fakeYTOAuth is a scripted YouTubeOAuth: it records calls and returns canned
// accounts so the service logic is tested without the network. Unlike the Spotify
// fake there is no PKCE (auth-code with client secret).
type fakeYTOAuth struct {
	state      string
	stateQueue []string // if set, NewState pops from here (multi-Begin tests)
	stateErr   error

	exchanged youtube.Account
	exchErr   error
	gotCode   string

	refreshed  youtube.Account
	refreshErr error
	gotRefresh string

	authURL   string
	gotScopes []string
}

func (f *fakeYTOAuth) AuthURL(state string, scopes []string) string {
	f.gotScopes = scopes
	return f.authURL
}
func (f *fakeYTOAuth) NewState() (string, error) {
	if len(f.stateQueue) > 0 {
		s := f.stateQueue[0]
		f.stateQueue = f.stateQueue[1:]
		return s, f.stateErr
	}
	return f.state, f.stateErr
}
func (f *fakeYTOAuth) Exchange(_ context.Context, code string) (youtube.Account, error) {
	f.gotCode = code
	return f.exchanged, f.exchErr
}
func (f *fakeYTOAuth) Refresh(_ context.Context, rt string) (youtube.Account, error) {
	f.gotRefresh = rt
	return f.refreshed, f.refreshErr
}

func TestYouTube_Begin_storesPendingAndReturnsURL(t *testing.T) {
	oauth := &fakeYTOAuth{state: "st8", authURL: "https://accounts.google.com/o/oauth2/v2/auth?x=1"}
	svc := NewYouTubeService(oauth, memory.NewYouTubeAccountRepo(), &movableClock{t: t0})

	url, err := svc.Begin(context.Background())
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if url != oauth.authURL {
		t.Fatalf("url = %q", url)
	}
	if svc.pending["st8"] == nil || svc.pending["st8"].State != "st8" {
		t.Fatalf("pending not stored: %+v", svc.pending)
	}
	if len(oauth.gotScopes) == 0 {
		t.Fatal("scopes not passed to AuthURL")
	}
}

func TestYouTube_Begin_stateError(t *testing.T) {
	oauth := &fakeYTOAuth{stateErr: errors.New("rng down")}
	svc := NewYouTubeService(oauth, memory.NewYouTubeAccountRepo(), &movableClock{t: t0})
	if _, err := svc.Begin(context.Background()); err == nil {
		t.Fatal("want state error")
	}
}

func TestYouTube_Complete_ok(t *testing.T) {
	oauth := &fakeYTOAuth{state: "st8", exchanged: connectedYTAccount(t0.Add(time.Hour))}
	repo := memory.NewYouTubeAccountRepo()
	svc := NewYouTubeService(oauth, repo, &movableClock{t: t0})

	if _, err := svc.Begin(context.Background()); err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := svc.Complete(context.Background(), "st8", "code123"); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if oauth.gotCode != "code123" {
		t.Fatalf("exchange got code=%q", oauth.gotCode)
	}
	acc, err := repo.Load(context.Background())
	if err != nil || !acc.Connected() {
		t.Fatalf("account not persisted: %v %+v", err, acc)
	}
	if len(svc.pending) != 0 {
		t.Fatal("pending must clear after complete")
	}
}

func TestYouTube_Complete_noPending(t *testing.T) {
	svc := NewYouTubeService(&fakeYTOAuth{}, memory.NewYouTubeAccountRepo(), &movableClock{t: t0})
	if err := svc.Complete(context.Background(), "s", "c"); err != youtube.ErrNoAccount {
		t.Fatalf("err = %v, want ErrNoAccount", err)
	}
}

func TestYouTube_Complete_expired(t *testing.T) {
	oauth := &fakeYTOAuth{state: "st8"}
	clk := &movableClock{t: t0}
	svc := NewYouTubeService(oauth, memory.NewYouTubeAccountRepo(), clk)
	_, _ = svc.Begin(context.Background())
	clk.t = t0.Add(11 * time.Minute) // past the 10m pending TTL
	if err := svc.Complete(context.Background(), "st8", "c"); err != youtube.ErrAuthExpired {
		t.Fatalf("err = %v, want ErrAuthExpired", err)
	}
	if len(svc.pending) != 0 {
		t.Fatal("expired pending must clear")
	}
}

func TestYouTube_Complete_stateMismatch(t *testing.T) {
	oauth := &fakeYTOAuth{state: "st8"}
	svc := NewYouTubeService(oauth, memory.NewYouTubeAccountRepo(), &movableClock{t: t0})
	_, _ = svc.Begin(context.Background())
	if err := svc.Complete(context.Background(), "WRONG", "c"); err != youtube.ErrStateMismatch {
		t.Fatalf("err = %v, want ErrStateMismatch", err)
	}
}

func TestYouTube_Complete_exchangeError(t *testing.T) {
	oauth := &fakeYTOAuth{state: "st8", exchErr: errors.New("google 400")}
	svc := NewYouTubeService(oauth, memory.NewYouTubeAccountRepo(), &movableClock{t: t0})
	_, _ = svc.Begin(context.Background())
	if err := svc.Complete(context.Background(), "st8", "c"); err == nil {
		t.Fatal("want exchange error")
	}
}

// TestYouTube_Complete_toleratesSecondBegin is the regression for the
// "oauth state mismatch" bug: two Begin() calls (retry / second tab) no longer
// invalidate the first authorize page — completing with EITHER state works.
func TestYouTube_Complete_toleratesSecondBegin(t *testing.T) {
	oauth := &fakeYTOAuth{stateQueue: []string{"first", "second"}, exchanged: connectedYTAccount(t0.Add(time.Hour))}
	repo := memory.NewYouTubeAccountRepo()
	svc := NewYouTubeService(oauth, repo, &movableClock{t: t0})

	if _, err := svc.Begin(context.Background()); err != nil { // pending["first"]
		t.Fatalf("Begin 1: %v", err)
	}
	if _, err := svc.Begin(context.Background()); err != nil { // pending["second"] — used to clobber "first"
		t.Fatalf("Begin 2: %v", err)
	}
	// Callback for the FIRST authorize page must still complete.
	if err := svc.Complete(context.Background(), "first", "code123"); err != nil {
		t.Fatalf("Complete(first) = %v, want nil", err)
	}
	if len(svc.pending) != 0 {
		t.Fatalf("pending must clear after complete, got %d", len(svc.pending))
	}
}

func TestYouTube_Status(t *testing.T) {
	repo := memory.NewYouTubeAccountRepo()
	svc := NewYouTubeService(&fakeYTOAuth{}, repo, &movableClock{t: t0})

	// Disconnected.
	st, err := svc.Status(context.Background())
	if err != nil || st.Connected || st.Premium {
		t.Fatalf("empty status = %+v err %v", st, err)
	}
	// Connected (premium best-effort false on YouTube).
	if err := repo.Save(context.Background(), connectedYTAccount(t0.Add(time.Hour))); err != nil {
		t.Fatalf("save: %v", err)
	}
	st, err = svc.Status(context.Background())
	if err != nil || !st.Connected || st.Premium {
		t.Fatalf("connected status = %+v err %v", st, err)
	}
}

func TestYouTube_Disconnect(t *testing.T) {
	repo := memory.NewYouTubeAccountRepo()
	_ = repo.Save(context.Background(), connectedYTAccount(t0.Add(time.Hour)))
	svc := NewYouTubeService(&fakeYTOAuth{}, repo, &movableClock{t: t0})
	if err := svc.Disconnect(context.Background()); err != nil {
		t.Fatalf("Disconnect: %v", err)
	}
	if _, err := repo.Load(context.Background()); err != youtube.ErrNoAccount {
		t.Fatalf("account still linked: %v", err)
	}
	// Idempotent.
	if err := svc.Disconnect(context.Background()); err != nil {
		t.Fatalf("second Disconnect: %v", err)
	}
}

func TestYouTube_AccessToken_noAccount(t *testing.T) {
	svc := NewYouTubeService(&fakeYTOAuth{}, memory.NewYouTubeAccountRepo(), &movableClock{t: t0})
	if _, err := svc.AccessToken(context.Background()); err != youtube.ErrNoAccount {
		t.Fatalf("err = %v, want ErrNoAccount", err)
	}
}

func TestYouTube_AccessToken_fresh(t *testing.T) {
	repo := memory.NewYouTubeAccountRepo()
	_ = repo.Save(context.Background(), connectedYTAccount(t0.Add(time.Hour)))
	oauth := &fakeYTOAuth{}
	svc := NewYouTubeService(oauth, repo, &movableClock{t: t0})
	tok, err := svc.AccessToken(context.Background())
	if err != nil || tok != "acc-tok" {
		t.Fatalf("tok = %q err %v", tok, err)
	}
	if oauth.gotRefresh != "" {
		t.Fatal("fresh token must not refresh")
	}
}

func TestYouTube_AccessToken_refreshes(t *testing.T) {
	repo := memory.NewYouTubeAccountRepo()
	_ = repo.Save(context.Background(), connectedYTAccount(t0.Add(30*time.Second))) // inside refresh window
	oauth := &fakeYTOAuth{refreshed: youtube.Account{AccessToken: "new-tok", RefreshToken: "ref-tok", ExpiresAt: t0.Add(time.Hour)}}
	svc := NewYouTubeService(oauth, repo, &movableClock{t: t0})
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

func TestYouTube_AccessToken_refreshError(t *testing.T) {
	repo := memory.NewYouTubeAccountRepo()
	_ = repo.Save(context.Background(), connectedYTAccount(t0.Add(30*time.Second)))
	oauth := &fakeYTOAuth{refreshErr: errors.New("google 401")}
	svc := NewYouTubeService(oauth, repo, &movableClock{t: t0})
	if _, err := svc.AccessToken(context.Background()); err == nil {
		t.Fatal("want refresh error")
	}
}

// --- paste-import: importar links colados sem conta ligada (0.2.17) ---

// unlinkedYTSvc é o cenário do paste-import: catálogo ligado (API key), NENHUMA
// conta Google conectada. Espelha o servidor em que o operador nunca fez login.
func unlinkedYTSvc(cat *fakeYTCatalog) (*YouTubeService, *memory.TrackRepo) {
	tracks := memory.New()
	svc := NewYouTubeService(&fakeYTOAuth{}, memory.NewYouTubeAccountRepo(), &movableClock{t: t0}).
		WithCatalog(cat, tracks)
	return svc, tracks
}

func TestYouTube_ImportLinks_NoAccountConnected(t *testing.T) {
	const id = "dQw4w9WgXcQ"
	cat := &fakeYTCatalog{videosItems: []youtube.CatalogItem{ytVideoItem(id, "Neon Skyline", "The Paper Kites", 210000)}}
	svc, tracks := unlinkedYTSvc(cat)

	got, err := svc.ImportLinks(context.Background(), "https://www.youtube.com/watch?v="+id)
	if err != nil {
		t.Fatalf("import sem conta deve funcionar: %v", err)
	}
	if len(got) != 1 || got[0].Title != "Neon Skyline" {
		t.Fatalf("tracks = %+v", got)
	}
	if got[0].Source != shared.SourceYouTube || got[0].Mixable {
		t.Fatalf("faixa YouTube deve ser não-mixável: %+v", got[0])
	}
	if len(cat.gotIDs) != 1 || cat.gotIDs[0] != id {
		t.Fatalf("catálogo recebeu ids = %v, want [%s]", cat.gotIDs, id)
	}
	if cat.gotToken != "" {
		t.Fatalf("token = %q, want vazio (paste-import não usa OAuth)", cat.gotToken)
	}
	all, _ := tracks.List(context.Background())
	if len(all) != 1 {
		t.Fatalf("biblioteca = %d faixas, want 1", len(all))
	}
}

// Regressão: arrastar pro deck um resultado achado no motor grátis passa por
// ImportTracks. Enquanto ele exigia conta, o operador buscava deslogado e levava
// "Conta do YouTube não conectada" ao soltar a faixa — busca livre inútil na prática.
// videos.list custa 1 unidade por lote e autentica pela API KEY: não há o que gatear.
func TestYouTube_ImportTracks_NoAccountConnected(t *testing.T) {
	const id = "dGFxdmuDA4A"
	cat := &fakeYTCatalog{videosItems: []youtube.CatalogItem{ytVideoItem(id, "Diário de um Detento", "Racionais MC's", 451000)}}
	svc, tracks := unlinkedYTSvc(cat)

	got, err := svc.ImportTracks(context.Background(), []string{id})
	if err != nil {
		t.Fatalf("import sem conta deve funcionar: %v", err)
	}
	if len(got) != 1 || got[0].Title != "Diário de um Detento" {
		t.Fatalf("tracks = %+v", got)
	}
	if got[0].Source != shared.SourceYouTube || got[0].Mixable {
		t.Fatalf("faixa YouTube deve ser não-mixável: %+v", got[0])
	}
	if cat.gotToken != "" {
		t.Fatalf("token = %q, want vazio (sem conta ligada)", cat.gotToken)
	}
	all, _ := tracks.List(context.Background())
	if len(all) != 1 {
		t.Fatalf("biblioteca = %d faixas, want 1", len(all))
	}
}

func TestYouTube_ImportPlaylist_NoAccountConnected(t *testing.T) {
	cat := &fakeYTCatalog{playlistItems: []youtube.CatalogItem{
		ytVideoItem("vid1", "Amber Waves", "Hollow Coves", 180000),
	}}
	svc, tracks := unlinkedYTSvc(cat)

	got, err := svc.ImportPlaylist(context.Background(), "pl9")
	if err != nil {
		t.Fatalf("import de playlist sem conta deve funcionar: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("tracks = %+v", got)
	}
	if cat.gotToken != "" {
		t.Fatalf("token = %q, want vazio (sem conta ligada)", cat.gotToken)
	}
	all, _ := tracks.List(context.Background())
	if len(all) != 1 {
		t.Fatalf("biblioteca = %d faixas, want 1", len(all))
	}
}

// Conta ligada segue repassando o token (paridade de assinatura — o adapter da
// Data API ignora, mas o caminho não pode regredir pra sempre-vazio).
func TestYouTube_ImportTracks_WithAccountForwardsToken(t *testing.T) {
	cat := &fakeYTCatalog{videosItems: []youtube.CatalogItem{ytVideoItem("vid1", "Amber Waves", "Hollow Coves", 180000)}}
	svc, _ := linkedYTSvc(t, &fakeYTOAuth{}, cat)
	if _, err := svc.ImportTracks(context.Background(), []string{"vid1"}); err != nil {
		t.Fatal(err)
	}
	if cat.gotToken == "" {
		t.Fatal("token vazio: conta ligada deve repassar o access token")
	}
}

func TestYouTube_ImportLinks_variosSeparadoresEDedupe(t *testing.T) {
	const a, b = "dQw4w9WgXcQ", "9bZkp7q19f0"
	cat := &fakeYTCatalog{videosItems: []youtube.CatalogItem{
		ytVideoItem(a, "A", "Canal A", 200000),
		ytVideoItem(b, "B", "Canal B", 180000),
	}}
	svc, _ := unlinkedYTSvc(cat)

	raw := "https://youtu.be/" + a + " ; https://www.youtube.com/watch?v=" + b + "\n" + a
	if _, err := svc.ImportLinks(context.Background(), raw); err != nil {
		t.Fatal(err)
	}
	if len(cat.gotIDs) != 2 || cat.gotIDs[0] != a || cat.gotIDs[1] != b {
		t.Fatalf("ids = %v, want [%s %s] sem duplicata", cat.gotIDs, a, b)
	}
}

func TestYouTube_ImportLinks_NoRecognizedLink(t *testing.T) {
	svc, _ := unlinkedYTSvc(&fakeYTCatalog{})
	if _, err := svc.ImportLinks(context.Background(), "isso aqui não é link nenhum"); err != youtube.ErrNoLinks {
		t.Fatalf("err = %v, want ErrNoLinks", err)
	}
}

func TestYouTube_ImportLinks_NoCatalog(t *testing.T) {
	svc := NewYouTubeService(&fakeYTOAuth{}, memory.NewYouTubeAccountRepo(), &movableClock{t: t0})
	if _, err := svc.ImportLinks(context.Background(), "https://youtu.be/dQw4w9WgXcQ"); err != youtube.ErrCatalogUnavailable {
		t.Fatalf("err = %v, want ErrCatalogUnavailable", err)
	}
}

func TestYouTube_ImportLinks_CatalogError(t *testing.T) {
	cat := &fakeYTCatalog{videosErr: errors.New("boom")}
	svc, _ := unlinkedYTSvc(cat)
	if _, err := svc.ImportLinks(context.Background(), "https://youtu.be/dQw4w9WgXcQ"); err == nil {
		t.Fatal("want erro do catálogo")
	}
}

// --- servidor sem cliente OAuth: só a API key configurada (0.2.17) ---

func TestYouTube_NoOAuth_ConnectUnavailableButSearchWorks(t *testing.T) {
	cat := &fakeYTCatalog{searchItems: []youtube.CatalogItem{ytVideoItem("vid1", "Neon", "Kites", 210000)}}
	svc := NewYouTubeService(nil, memory.NewYouTubeAccountRepo(), &movableClock{t: t0}).
		WithCatalog(cat, memory.New())
	ctx := context.Background()

	if _, err := svc.Begin(ctx); err != youtube.ErrOAuthUnavailable {
		t.Fatalf("Begin err = %v, want ErrOAuthUnavailable", err)
	}
	if err := svc.Complete(ctx, "s", "c"); err != youtube.ErrOAuthUnavailable {
		t.Fatalf("Complete err = %v, want ErrOAuthUnavailable", err)
	}
	if _, err := svc.AccessToken(ctx); err != youtube.ErrOAuthUnavailable {
		t.Fatalf("AccessToken err = %v, want ErrOAuthUnavailable", err)
	}
	// sem OAuth o portão de login seria beco sem saída → busca roda desligada.
	items, err := svc.Search(ctx, "neon", youtube.KindVideo, youtube.BackendAPI)
	if err != nil {
		t.Fatalf("Search sem OAuth deve rodar: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("items = %+v", items)
	}
	// status continua legível (desconectado, sem erro)
	st, err := svc.Status(ctx)
	if err != nil || st.Connected {
		t.Fatalf("status = %+v err = %v", st, err)
	}
}

// --- seletor de backend da busca (0.2.17): Data API e InnerTube lado a lado ---

// freeYTSvc devolve um serviço com OS DOIS backends ligados: Data API (gated por
// login) e o gratuito. Nada é substituído — o operador escolhe na chamada.
func freeYTSvc(free, api *fakeYTCatalog) *YouTubeService {
	return NewYouTubeService(&fakeYTOAuth{}, memory.NewYouTubeAccountRepo(), &movableClock{t: t0}).
		WithCatalog(api, memory.New()).
		WithFreeCatalog(free)
}

// O backend gratuito é anônimo por construção: roda sem conta e NÃO pode encostar
// no portão de login (que existe só p/ proteger a cota da Data API).
func TestYouTube_Search_FreeBackend_NoAccount(t *testing.T) {
	free := &fakeYTCatalog{searchItems: []youtube.CatalogItem{ytVideoItem("vid1", "Neon", "Kites", 210000)}}
	api := &fakeYTCatalog{}
	svc := freeYTSvc(free, api)

	items, err := svc.Search(context.Background(), "neon", youtube.KindVideo, youtube.BackendFree)
	if err != nil {
		t.Fatalf("busca grátis deve rodar deslogada: %v", err)
	}
	if len(items) != 1 || items[0].Title != "Neon" {
		t.Fatalf("items = %+v", items)
	}
	if free.gotToken != "" {
		t.Fatalf("backend grátis recebeu token %q — não deve autenticar", free.gotToken)
	}
	if free.gotQuery != "neon" || free.gotKind != youtube.KindVideo {
		t.Fatalf("query=%q kind=%q", free.gotQuery, free.gotKind)
	}
	if api.gotQuery != "" {
		t.Fatal("Data API foi chamada no caminho grátis")
	}
}

// Cliente que não conhece o seletor manda "" — cai no grátis, que funciona deslogado.
func TestYouTube_Search_backendDesconhecido_caiNoFree(t *testing.T) {
	for _, backend := range []string{"", "xpto"} {
		free := &fakeYTCatalog{searchItems: []youtube.CatalogItem{ytVideoItem("vid1", "Neon", "Kites", 210000)}}
		svc := freeYTSvc(free, &fakeYTCatalog{})
		if _, err := svc.Search(context.Background(), "neon", youtube.KindVideo, backend); err != nil {
			t.Fatalf("backend %q: %v", backend, err)
		}
		if free.gotQuery != "neon" {
			t.Fatalf("backend %q não caiu no grátis", backend)
		}
	}
}

// Escolhendo a Data API explicitamente, o portão de login continua valendo.
func TestYouTube_Search_backendAPI_mantemPortaoDeLogin(t *testing.T) {
	free := &fakeYTCatalog{}
	api := &fakeYTCatalog{searchItems: []youtube.CatalogItem{ytVideoItem("vid1", "Neon", "Kites", 210000)}}
	svc := freeYTSvc(free, api)

	if _, err := svc.Search(context.Background(), "neon", youtube.KindVideo, youtube.BackendAPI); err != youtube.ErrNoAccount {
		t.Fatalf("err = %v, want ErrNoAccount", err)
	}
	if free.gotQuery != "" {
		t.Fatal("backend grátis foi chamado com backend=api")
	}

	svc2, _ := linkedYTSvc(t, &fakeYTOAuth{}, api)
	svc2 = svc2.WithFreeCatalog(free)
	items, err := svc2.Search(context.Background(), "neon", youtube.KindVideo, youtube.BackendAPI)
	if err != nil {
		t.Fatalf("Search = %v", err)
	}
	if len(items) != 1 || api.gotToken != "acc-tok" {
		t.Fatalf("items = %+v token = %q", items, api.gotToken)
	}
}

// Servidor sem o backend grátis: pedido "free" degrada p/ a Data API em vez de sumir.
func TestYouTube_Search_NoFree_FallsBackToDataAPI(t *testing.T) {
	api := &fakeYTCatalog{searchItems: []youtube.CatalogItem{ytVideoItem("vid1", "Neon", "Kites", 210000)}}
	svc, _ := linkedYTSvc(t, &fakeYTOAuth{}, api)

	items, err := svc.Search(context.Background(), "neon", youtube.KindVideo, youtube.BackendFree)
	if err != nil {
		t.Fatalf("Search = %v", err)
	}
	if len(items) != 1 || api.gotToken != "acc-tok" {
		t.Fatalf("items = %+v token = %q", items, api.gotToken)
	}
}

// O status carrega o flag que a UI usa p/ mostrar (ou esconder) o chaveador.
func TestYouTube_Status_freeSearch(t *testing.T) {
	ctx := context.Background()

	// (a) sem conta, com backend grátis
	svc := freeYTSvc(&fakeYTCatalog{}, &fakeYTCatalog{})
	st, err := svc.Status(ctx)
	if err != nil || !st.FreeSearch || st.Connected {
		t.Fatalf("sem conta: status = %+v err = %v", st, err)
	}

	// (b) conta conectada, com backend grátis
	linked, _ := linkedYTSvc(t, &fakeYTOAuth{}, &fakeYTCatalog{})
	linked = linked.WithFreeCatalog(&fakeYTCatalog{})
	if st, err = linked.Status(ctx); err != nil || !st.FreeSearch || !st.Connected {
		t.Fatalf("conectado: status = %+v err = %v", st, err)
	}

	// (c) servidor sem backend grátis
	only, _ := linkedYTSvc(t, &fakeYTOAuth{}, &fakeYTCatalog{})
	if st, err = only.Status(ctx); err != nil || st.FreeSearch {
		t.Fatalf("sem free: status = %+v err = %v", st, err)
	}
}
