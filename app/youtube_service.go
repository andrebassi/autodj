package app

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"sync"
	"time"

	"autodj/domain/library"
	"autodj/domain/shared"
	"autodj/domain/youtube"
	"autodj/ports/inbound"
	"autodj/ports/outbound"
)

// youtubeScopes is the minimal set the AutoDJ needs: youtube.readonly lets us
// read the linked channel (for ChannelTitle) and openid/email/profile identify
// the Google account on the consent screen. Search/import use the API KEY, not
// this token, so no broad data scope is required.
var youtubeScopes = []string{
	"https://www.googleapis.com/auth/youtube.readonly",
	"openid",
	"email",
	"profile",
}

// YouTubeService runs the OAuth session that links the operator to their Google/
// YouTube account — a faithful mirror of SpotifyService. It drives the auth-code
// flow (WITH client secret, not PKCE) through the outbound gateway, persists the
// account via the single-row repo, and hands out a fresh access token (refreshing
// when near expiry). The pending flow is held in memory (one operator, home use)
// and cleared once completed. Imports land as non-mixable SourceYouTube tracks.
type YouTubeService struct {
	oauth   outbound.YouTubeOAuth
	repo    outbound.YouTubeAccountRepo
	clock   outbound.Clock
	catalog outbound.YouTubeCatalog // optional; nil → search/import unavailable
	free    outbound.YouTubeCatalog // optional; free (InnerTube) search backend
	tracks  outbound.TrackRepo      // catalog target for imports

	mu      sync.Mutex
	pending map[string]*youtube.PendingAuth // keyed by state; tolerates retries / multiple tabs
}

// maxPendingAuth caps concurrent in-flight OAuth attempts so the pending map can
// never grow without bound (re-clicks, abandoned tabs). Well above real usage.
const maxPendingAuth = 16

// NewYouTubeService builds the service. clock is injected for deterministic
// token-expiry tests.
func NewYouTubeService(oauth outbound.YouTubeOAuth, repo outbound.YouTubeAccountRepo, clock outbound.Clock) *YouTubeService {
	return &YouTubeService{oauth: oauth, repo: repo, clock: clock}
}

// WithCatalog enables search + import: the Data API gateway and the catalog repo
// imports land in. Returns the service for chaining. Without it, Search/
// ImportTracks/ImportPlaylist return youtube.ErrCatalogUnavailable.
func (s *YouTubeService) WithCatalog(catalog outbound.YouTubeCatalog, tracks outbound.TrackRepo) *YouTubeService {
	s.catalog = catalog
	s.tracks = tracks
	return s
}

// WithFreeCatalog wires the free search backend (InnerTube): no API key, no OAuth
// and no quota. It only ever answers Search — import/playlist expansion stay on
// the Data API catalog, which knows exact durations and drops non-embeddable
// videos. Optional: without it every search falls back to the Data API.
func (s *YouTubeService) WithFreeCatalog(free outbound.YouTubeCatalog) *YouTubeService {
	s.free = free
	return s
}

// Search queries the YouTube catalog through the backend the operator picked.
//
// BackendFree (the default, including for a client that sends nothing) answers
// from the InnerTube gateway: no account, no API key, no quota — so it never
// touches the login gate below.
//
// BackendAPI goes through the Data API. That one authenticates with the API key
// (the token argument is ignored downstream), but the login gate is kept as a
// QUOTA SHIELD: search costs ~100 units of a ~10k/day budget, so an unlinked
// visitor cannot burn the station's quota. When this server has no OAuth client
// configured at all there is no way to link an account, so the gate would be a
// dead end — in that case search runs unlinked.
//
// Either backend needs its gateway wired (ErrCatalogUnavailable); with the free
// one absent, a free request falls back to the Data API path.
func (s *YouTubeService) Search(ctx context.Context, query, kind, backend string) ([]youtube.CatalogItem, error) {
	// Free backend first: it is anonymous by construction, so it must NOT touch
	// the OAuth gate below — that gate exists only to protect the Data API's
	// shared daily quota, which the free path never spends.
	if youtube.NormalizeBackend(backend) == youtube.BackendFree && s.free != nil {
		return s.free.Search(ctx, "", query, kind)
	}
	if s.catalog == nil {
		return nil, youtube.ErrCatalogUnavailable
	}
	var token string
	if s.oauth != nil {
		var err error
		if token, err = s.AccessToken(ctx); err != nil {
			return nil, err
		}
	}
	return s.catalog.Search(ctx, token, query, kind)
}

// ImportLinks imports videos from free-form pasted text (links separated by ";",
// newlines or commas) WITHOUT requiring a linked account: resolving ids costs 1
// Data API unit per batch, so there is no quota reason to gate it, and forcing a
// Google login just to queue a link the operator already has is friction for
// nothing.
//
// Text with no recognizable video link → youtube.ErrNoLinks. Entries past
// youtube.MaxImportLinks are dropped (the Data API caps videos.list at 50 ids);
// the caller learns the accepted count from len(result). Non-embeddable videos
// are filtered by the catalog, so a link may parse fine and still not import —
// it would never produce audio in the IFrame player.
func (s *YouTubeService) ImportLinks(ctx context.Context, raw string) ([]library.Track, error) {
	if s.catalog == nil {
		return nil, youtube.ErrCatalogUnavailable
	}
	ids := youtube.ParseLinks(raw)
	if len(ids) == 0 {
		return nil, youtube.ErrNoLinks
	}
	items, err := s.catalog.Videos(ctx, "", ids)
	if err != nil {
		return nil, err
	}
	return s.importItems(ctx, items)
}

// ImportTracks resolves video URIs/ids to metadata and imports them as non-mixable
// YouTube tracks (the source matrix forces mixable=false, pcm=false — embed audio
// never reaches the playout bus). Returns the created tracks.
//
// NÃO exige conta conectada: videos.list custa 1 unidade por lote (≤50 ids) e
// autentica pela API KEY, então não há razão de cota pra gatear — mesma lógica de
// ImportLinks. Gatear aqui quebrava o motor de busca grátis: o operador achava a
// faixa deslogado e levava "conta não conectada" ao arrastar pro deck.
func (s *YouTubeService) ImportTracks(ctx context.Context, uris []string) ([]library.Track, error) {
	if s.catalog == nil {
		return nil, youtube.ErrCatalogUnavailable
	}
	items, err := s.catalog.Videos(ctx, s.bestEffortToken(ctx), uris)
	if err != nil {
		return nil, err
	}
	return s.importItems(ctx, items)
}

// ImportPlaylist imports every video of a playlist as non-mixable YouTube tracks.
// Sem gate de conta pelo mesmo motivo de ImportTracks (playlistItems.list = 1
// unidade por página, autenticada pela API KEY).
func (s *YouTubeService) ImportPlaylist(ctx context.Context, playlistID string) ([]library.Track, error) {
	if s.catalog == nil {
		return nil, youtube.ErrCatalogUnavailable
	}
	items, err := s.catalog.PlaylistItems(ctx, s.bestEffortToken(ctx), playlistID)
	if err != nil {
		return nil, err
	}
	return s.importItems(ctx, items)
}

// bestEffortToken devolve o access token quando há conta conectada e "" quando não
// há (ou quando o refresh falha). O adapter da Data API ignora o argumento — tudo
// autentica pela API KEY —, então isto é só paridade de assinatura: nenhuma falha
// de OAuth pode derrubar um import que não depende de OAuth.
func (s *YouTubeService) bestEffortToken(ctx context.Context) string {
	token, err := s.AccessToken(ctx)
	if err != nil {
		return ""
	}
	return token
}

// importItems turns catalog items into validated SourceYouTube tracks and upserts
// them (id derived from the URI, so re-importing dedupes). Playlist entries are
// skipped — only video URIs become tracks.
func (s *YouTubeService) importItems(ctx context.Context, items []youtube.CatalogItem) ([]library.Track, error) {
	out := make([]library.Track, 0, len(items))
	for _, it := range items {
		if it.URI == "" || it.Kind == youtube.KindPlaylist {
			continue
		}
		dur := time.Duration(it.DurationMs) * time.Millisecond
		if dur <= 0 {
			continue // metadata-less entry (live/unavailable) — skip
		}
		t, err := library.NewTrack(youtubeID(it.URI), shared.SourceYouTube, it.Title, dur)
		if err != nil {
			return nil, err
		}
		t.Artist = it.Artist
		t.Album = it.Album
		t.ArtworkURL = it.ArtworkURL
		t.YouTubeURI = it.URI
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

// youtubeID derives a stable catalog id from a YouTube URI so re-imports upsert
// the same row instead of duplicating.
func youtubeID(uri string) string {
	sum := sha1.Sum([]byte(uri))
	return hex.EncodeToString(sum[:])
}

// Begin starts an auth-code flow: generates state, stores the pending auth, and
// returns the authorize URL for the browser to navigate to. Unlike Spotify there
// is no PKCE (the client secret authenticates the token exchange).
func (s *YouTubeService) Begin(ctx context.Context) (string, error) {
	if s.oauth == nil {
		return "", youtube.ErrOAuthUnavailable
	}
	state, err := s.oauth.NewState()
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	s.rememberPendingLocked(state)
	s.mu.Unlock()
	return s.oauth.AuthURL(state, youtubeScopes), nil
}

// rememberPendingLocked records a fresh pending flow keyed by its state, pruning
// expired entries first. Keying by state (instead of a single slot) means a
// re-clicked "Connect" — or a second open tab — no longer invalidates an
// already-open authorize page: the callback for ANY still-valid state completes.
// Caller holds s.mu.
func (s *YouTubeService) rememberPendingLocked(state string) {
	if s.pending == nil {
		s.pending = make(map[string]*youtube.PendingAuth)
	}
	now := s.clock.Now()
	for k, p := range s.pending {
		if p.Expired(now) {
			delete(s.pending, k)
		}
	}
	if len(s.pending) >= maxPendingAuth {
		s.pending = make(map[string]*youtube.PendingAuth) // runaway guard: reset
	}
	s.pending[state] = &youtube.PendingAuth{State: state, CreatedAt: now}
}

// Complete validates the callback state against the pending flow, exchanges the
// code for tokens and persists the account. Rejects a missing/expired/mismatched
// pending flow without touching the store.
func (s *YouTubeService) Complete(ctx context.Context, state, code string) error {
	if s.oauth == nil {
		return youtube.ErrOAuthUnavailable
	}
	s.mu.Lock()
	if len(s.pending) == 0 {
		s.mu.Unlock()
		return youtube.ErrNoAccount
	}
	p := s.pending[state]
	s.mu.Unlock()

	if p == nil {
		return youtube.ErrStateMismatch
	}
	if p.Expired(s.clock.Now()) {
		s.mu.Lock()
		delete(s.pending, state)
		s.mu.Unlock()
		return youtube.ErrAuthExpired
	}

	acc, err := s.oauth.Exchange(ctx, code)
	if err != nil {
		return err
	}
	if err := s.repo.Save(ctx, acc); err != nil {
		return err
	}
	s.clearPending()
	return nil
}

// Status reports whether an account is connected, its best-effort Premium, and
// whether the free search backend is wired — the panel needs that last one to know
// it can offer the backend switch (and to search at all while logged out). An
// absent account (ErrNoAccount) reads back as disconnected, not an error.
func (s *YouTubeService) Status(ctx context.Context) (inbound.YouTubeStatus, error) {
	free := s.free != nil
	// OAuth ausente (sem CLIENT_ID/SECRET) → o motor de busca "Minha conta" não tem
	// como funcionar: connect responde 503. Publicar isso deixa a UI esconder esse
	// motor em vez de oferecer um caminho que termina em erro.
	oauth := s.oauth != nil
	base := inbound.YouTubeStatus{FreeSearch: free, OAuth: oauth}
	acc, err := s.repo.Load(ctx)
	if err == youtube.ErrNoAccount {
		return base, nil
	}
	if err != nil {
		return base, err
	}
	base.Connected, base.Premium = acc.Connected(), acc.IsPremium()
	return base, nil
}

// Disconnect unlinks the account and drops any pending flow. Idempotent.
func (s *YouTubeService) Disconnect(ctx context.Context) error {
	s.clearPending()
	return s.repo.Delete(ctx)
}

// AccessToken returns a valid access token, refreshing transparently when the
// stored one is near expiry and persisting the refreshed account. Google may omit
// a new refresh token — the adapter preserves the old one.
func (s *YouTubeService) AccessToken(ctx context.Context) (string, error) {
	if s.oauth == nil {
		return "", youtube.ErrOAuthUnavailable
	}
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

func (s *YouTubeService) clearPending() {
	s.mu.Lock()
	s.pending = nil
	s.mu.Unlock()
}
