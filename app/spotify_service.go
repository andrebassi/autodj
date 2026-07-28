package app

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"strconv"
	"sync"
	"time"

	"autodj/domain/library"
	"autodj/domain/shared"
	"autodj/domain/spotify"
	"autodj/ports/inbound"
	"autodj/ports/outbound"
)

// spotifyScopes is the minimal set the AutoDJ needs: read-private detects
// Premium, streaming + playback prepare the Web Playback SDK (Story 6.3), and
// the two playlist-read scopes let us import the operator's OWN private and
// collaborative playlists (GET /playlists/{id}/tracks 403s without them — even
// for playlists you own, if they aren't public).
var spotifyScopes = []string{
	"user-read-private",
	"user-read-email",
	"streaming",
	"user-read-playback-state",
	"user-modify-playback-state",
	"playlist-read-private",
	"playlist-read-collaborative",
}

// SpotifyService runs the OAuth session that links the operator to Spotify. It
// drives the PKCE flow through the outbound gateway, persists the account via
// the single-row repo, and hands out a fresh access token (refreshing when near
// expiry). The pending flow is held in memory (one operator, home use) and
// cleared once completed. It creates no Track — the two-engine split is enforced
// when 6.2 imports metadata as non-mixable.
type SpotifyService struct {
	oauth   outbound.SpotifyOAuth
	repo    outbound.SpotifyAccountRepo
	clock   outbound.Clock
	catalog outbound.SpotifyCatalog // optional (Story 6.2); nil → search/import unavailable
	tracks  outbound.TrackRepo      // catalog target for imports

	mu      sync.Mutex
	pending map[string]*spotify.PendingAuth // keyed by state; tolerates retries / multiple tabs
}

// NewSpotifyService builds the service. clock is injected for deterministic
// token-expiry tests.
func NewSpotifyService(oauth outbound.SpotifyOAuth, repo outbound.SpotifyAccountRepo, clock outbound.Clock) *SpotifyService {
	return &SpotifyService{oauth: oauth, repo: repo, clock: clock}
}

// WithCatalog enables search + import (Story 6.2): the Web API gateway and the
// catalog repo imports land in. Returns the service for chaining. Without it,
// Search/ImportTracks/ImportPlaylist return spotify.ErrCatalogUnavailable.
func (s *SpotifyService) WithCatalog(catalog outbound.SpotifyCatalog, tracks outbound.TrackRepo) *SpotifyService {
	s.catalog = catalog
	s.tracks = tracks
	return s
}

// Search queries the Spotify catalog, refreshing the token transparently. Needs
// a linked account (ErrNoAccount) and a wired gateway (spotify.ErrCatalogUnavailable).
func (s *SpotifyService) Search(ctx context.Context, query, kind string) ([]spotify.CatalogItem, error) {
	if s.catalog == nil {
		return nil, spotify.ErrCatalogUnavailable
	}
	token, err := s.AccessToken(ctx)
	if err != nil {
		return nil, err
	}
	return s.catalog.Search(ctx, token, query, kind)
}

// ImportTracks resolves track URIs to metadata and imports them as non-mixable
// Spotify tracks (the source matrix forces mixable=false, pcm=false — DRM never
// reaches the playout bus). Returns the created tracks.
func (s *SpotifyService) ImportTracks(ctx context.Context, uris []string) ([]library.Track, error) {
	if s.catalog == nil {
		return nil, spotify.ErrCatalogUnavailable
	}
	token, err := s.AccessToken(ctx)
	if err != nil {
		return nil, err
	}
	items, err := s.catalog.Tracks(ctx, token, uris)
	if err != nil {
		return nil, err
	}
	return s.importItems(ctx, items)
}

// ImportPlaylist imports every track of a playlist as non-mixable Spotify tracks.
func (s *SpotifyService) ImportPlaylist(ctx context.Context, playlistID string) ([]library.Track, error) {
	if s.catalog == nil {
		return nil, spotify.ErrCatalogUnavailable
	}
	token, err := s.AccessToken(ctx)
	if err != nil {
		return nil, err
	}
	items, err := s.catalog.PlaylistTracks(ctx, token, playlistID)
	if err != nil {
		return nil, err
	}
	return s.importItems(ctx, items)
}

// importItems turns catalog items into validated SourceSpotify tracks and
// upserts them (id derived from the URI, so re-importing dedupes). Playlist
// entries are skipped — only track URIs become tracks.
func (s *SpotifyService) importItems(ctx context.Context, items []spotify.CatalogItem) ([]library.Track, error) {
	out := make([]library.Track, 0, len(items))
	for _, it := range items {
		if it.URI == "" || it.Kind == spotify.KindPlaylist {
			continue
		}
		dur := time.Duration(it.DurationMs) * time.Millisecond
		if dur <= 0 {
			continue // metadata-less entry (unavailable/local track) — skip
		}
		t, err := library.NewTrack(spotifyID(it.URI), shared.SourceSpotify, it.Title, dur)
		if err != nil {
			return nil, err
		}
		t.Artist = it.Artist
		t.Album = it.Album
		t.ArtworkURL = it.ArtworkURL
		t.SpotifyURI = it.URI
		if err := t.Validate(); err != nil {
			return nil, err
		}
		if err := s.tracks.Upsert(ctx, t); err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, nil
}

// spotifyID derives a stable catalog id from a Spotify URI so re-imports upsert
// the same row instead of duplicating.
func spotifyID(uri string) string {
	sum := sha1.Sum([]byte(uri))
	return hex.EncodeToString(sum[:])
}

// Begin starts a PKCE flow: generates state + verifier/challenge, stores the
// pending auth, and returns the authorize URL for the browser to navigate to.
func (s *SpotifyService) Begin(ctx context.Context) (string, error) {
	state, err := s.oauth.NewState()
	if err != nil {
		return "", err
	}
	verifier, challenge, err := s.oauth.NewPKCE()
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	s.rememberPendingLocked(state, verifier)
	s.mu.Unlock()
	return s.oauth.AuthURL(state, challenge, spotifyScopes), nil
}

// rememberPendingLocked records a fresh pending flow keyed by its state, pruning
// expired entries first. Keying by state (instead of a single slot) means a
// re-clicked "Connect" — or a second open tab — no longer invalidates an
// already-open authorize page: the callback for ANY still-valid state completes.
// Caller holds s.mu.
func (s *SpotifyService) rememberPendingLocked(state, verifier string) {
	if s.pending == nil {
		s.pending = make(map[string]*spotify.PendingAuth)
	}
	now := s.clock.Now()
	for k, p := range s.pending {
		if p.Expired(now) {
			delete(s.pending, k)
		}
	}
	if len(s.pending) >= maxPendingAuth {
		s.pending = make(map[string]*spotify.PendingAuth) // runaway guard: reset
	}
	s.pending[state] = &spotify.PendingAuth{State: state, Verifier: verifier, CreatedAt: now}
}

// Complete validates the callback state against the pending flow, exchanges the
// code for tokens and persists the account. Rejects a missing/expired/mismatched
// pending flow without touching the store.
func (s *SpotifyService) Complete(ctx context.Context, state, code string) error {
	s.mu.Lock()
	if len(s.pending) == 0 {
		s.mu.Unlock()
		return spotify.ErrNoAccount
	}
	p := s.pending[state]
	s.mu.Unlock()

	if p == nil {
		return spotify.ErrStateMismatch
	}
	if p.Expired(s.clock.Now()) {
		s.mu.Lock()
		delete(s.pending, state)
		s.mu.Unlock()
		return spotify.ErrAuthExpired
	}

	acc, err := s.oauth.Exchange(ctx, code, p.Verifier)
	if err != nil {
		return err
	}
	if err := s.repo.Save(ctx, acc); err != nil {
		return err
	}
	s.clearPending()
	return nil
}

// Status reports whether an account is connected and Premium. An absent account
// (ErrNoAccount) reads back as disconnected, not an error.
func (s *SpotifyService) Status(ctx context.Context) (inbound.SpotifyStatus, error) {
	acc, err := s.repo.Load(ctx)
	if err == spotify.ErrNoAccount {
		return inbound.SpotifyStatus{}, nil
	}
	if err != nil {
		return inbound.SpotifyStatus{}, err
	}
	return inbound.SpotifyStatus{Connected: acc.Connected(), Premium: acc.Premium()}, nil
}

// Revalidate força uma renovação do token AGORA e regrava a conta, mesmo que o
// token atual ainda não esteja perto de expirar.
//
// Existe por causa de um caso real: o SDK conectava, carregava a faixa no device e
// nunca tocava — sem erro nenhum — porque o Spotify havia passado a recusar o
// token (403 em /v1/me). Como `Product` (o selo Premium) só é reconsultado quando
// o token é renovado, o painel seguia mostrando "Conectado · Premium" enquanto
// nada tocava. Chamar isto no momento da falha traz o estado REAL: ou o token é
// renovado e o Premium se confirma (aí a culpa é outra), ou a renovação falha /
// o produto volta vazio, e a UI pode dizer o que de fato está acontecendo.
//
// Não é chamado em polling: renovar token à toa gasta rate limit do Spotify.
func (s *SpotifyService) Revalidate(ctx context.Context) (inbound.SpotifyStatus, error) {
	acc, err := s.repo.Load(ctx)
	if err != nil {
		return inbound.SpotifyStatus{}, err
	}
	refreshed, err := s.oauth.Refresh(ctx, acc.RefreshToken)
	if err != nil {
		// token morto: reporta desconectado em vez de mentir "conectado".
		return inbound.SpotifyStatus{}, err
	}
	if err := s.repo.Save(ctx, refreshed); err != nil {
		return inbound.SpotifyStatus{}, err
	}
	return inbound.SpotifyStatus{Connected: refreshed.Connected(), Premium: refreshed.Premium()}, nil
}

// Disconnect unlinks the account and drops any pending flow. Idempotent.
func (s *SpotifyService) Disconnect(ctx context.Context) error {
	s.clearPending()
	return s.repo.Delete(ctx)
}

// AccessToken returns a valid access token, refreshing transparently when the
// stored one is near expiry and persisting the refreshed account. Spotify may
// omit a new refresh token — the adapter preserves the old one.
func (s *SpotifyService) AccessToken(ctx context.Context) (string, error) {
	acc, err := s.repo.Load(ctx)
	if err != nil {
		return "", err
	}
	if !acc.NeedsRefresh(s.clock.Now()) {
		return acc.AccessToken, nil
	}
	refreshed, err := s.oauth.Refresh(ctx, acc.RefreshToken)
	if err != nil {
		return "", err
	}
	if err := s.repo.Save(ctx, refreshed); err != nil {
		return "", err
	}
	return refreshed.AccessToken, nil
}

// Diagnose roda a cadeia de verificação do Spotify e devolve um passo a passo
// legível. É a resposta a uma investigação de horas em que TODOS os sinais normais
// diziam "está tudo bem": o SDK conectava, o device registrava, o comando de tocar
// respondia 204 e nenhum evento de erro era emitido — só não saía som. O que
// denunciava a causa (conta fora da allowlist de um app em Development Mode) era um
// 403 no perfil, e só o SERVIDOR consegue enxergar isso: do browser o mesmo
// endpoint responde 403 por outro motivo, o que despista.
//
// A ordem dos degraus é a ordem causal: o primeiro que falha é a causa; os de baixo
// são consequência e nem chegam a ser executados.
func (s *SpotifyService) Diagnose(ctx context.Context) (inbound.SpotifyDiagnosis, error) {
	d := inbound.SpotifyDiagnosis{}
	add := func(key, label string, ok bool, detail, hint string) {
		d.Steps = append(d.Steps, inbound.SpotifyStep{Key: key, Label: label, OK: ok, Detail: detail, Hint: hint})
	}
	finish := func(verdict string) (inbound.SpotifyDiagnosis, error) {
		d.Verdict = verdict
		d.OK = verdict == ""
		return d, nil
	}

	// 1) existe conta ligada?
	acc, err := s.repo.Load(ctx)
	if err != nil {
		add("conta", "Conta conectada", false, "nenhuma conta ligada", "Clique em “Conectar conta”.")
		return finish("Nenhuma conta do Spotify está conectada.")
	}
	add("conta", "Conta conectada", true, "sessão encontrada", "")

	// 2) o token renova? (é o que mantém a sessão viva por horas)
	refreshed, err := s.oauth.Refresh(ctx, acc.RefreshToken)
	if err != nil {
		add("token", "Sessão válida", false, "a renovação falhou: "+err.Error(),
			"Reconecte a conta (Desconectar → Conectar).")
		return finish("A sessão expirou e não renova. Reconecte a conta.")
	}
	if err := s.repo.Save(ctx, refreshed); err != nil {
		return d, err
	}
	add("token", "Sessão válida", true, "token renovado agora", "")

	// 3) o Spotify aceita esse token pra ler o perfil? AQUI mora o 403 da allowlist.
	p, err := s.oauth.Probe(ctx, refreshed.AccessToken)
	if err != nil {
		add("perfil", "Conta autorizada no app", false, "não deu pra consultar: "+err.Error(),
			"Verifique a conexão de rede do servidor.")
		return finish("Não foi possível falar com o Spotify.")
	}
	switch p.ProfileStatus {
	case 200:
		add("perfil", "Conta autorizada no app", true, "perfil lido (plano: "+planoOuDesconhecido(p.Product)+")", "")
	case 403:
		add("perfil", "Conta autorizada no app", false, "HTTP 403 — o Spotify recusou ler o perfil",
			"O app está em Development Mode e esta conta não está liberada. Adicione-a em developer.spotify.com → seu app → User Management, depois reconecte a conta aqui.")
		return finish("Conta fora da lista de usuários do app (Development Mode).")
	case 401:
		add("perfil", "Conta autorizada no app", false, "HTTP 401 — token recusado",
			"Reconecte a conta (Desconectar → Conectar).")
		return finish("O Spotify recusou o token. Reconecte a conta.")
	default:
		add("perfil", "Conta autorizada no app", false, "HTTP "+itoa(p.ProfileStatus), "Tente de novo em alguns minutos.")
		return finish("O Spotify respondeu de forma inesperada ao perfil.")
	}

	// 4) Premium — sem ele o player do navegador não toca, por regra do Spotify.
	if refreshed.Product != "" && !refreshed.Premium() {
		add("premium", "Plano Premium", false, "plano atual: "+planoOuDesconhecido(refreshed.Product),
			"O player do navegador (Web Playback SDK) exige Premium.")
		return finish("A conta não tem Premium ativo.")
	}
	add("premium", "Plano Premium", true, "Premium ativo", "")

	// 5) o device do navegador apareceu pro Spotify?
	if p.DevicesStatus != 200 {
		add("device", "Aparelho registrado", false, "HTTP "+itoa(p.DevicesStatus), "Tente de novo em alguns minutos.")
		return finish("Não deu pra listar os aparelhos.")
	}
	if len(p.Devices) == 0 {
		add("device", "Aparelho registrado", false, "nenhum aparelho visível",
			"Recarregue esta página com a aba em foco — o player do navegador se registra ao abrir.")
		return finish("O player do navegador não está registrado no Spotify.")
	}
	detalhe := itoa(len(p.Devices)) + " aparelho(s)"
	if p.ActiveDevice != "" {
		detalhe += " · no ar: " + p.ActiveDevice
	} else {
		detalhe += " · nenhum ativo"
	}
	add("device", "Aparelho registrado", true, detalhe, "")

	return finish("")
}

func planoOuDesconhecido(p string) string {
	if p == "" {
		return "desconhecido"
	}
	return p
}

func itoa(n int) string { return strconv.Itoa(n) }

func (s *SpotifyService) clearPending() {
	s.mu.Lock()
	s.pending = nil
	s.mu.Unlock()
}
