// Package httpapi is the REST driving adapter. It translates HTTP to inbound
// use-case calls and never contains business logic.
package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"autodj/domain/aidj"
	"autodj/domain/cartwall"
	"autodj/domain/identity"
	"autodj/domain/library"
	"autodj/domain/linein"
	"autodj/domain/mixing"
	"autodj/domain/operatorprofile"
	"autodj/domain/rds"
	"autodj/domain/requests"
	"autodj/domain/schedule"
	"autodj/domain/social"
	"autodj/domain/spotify"
	"autodj/domain/staffschedule"
	"autodj/domain/youtube"
	"autodj/ports/inbound"
	"autodj/ports/outbound"
)

// maxUploadBytes caps a single uploaded media file (256 MiB — generous for
// lossless audio, bounded to avoid unbounded memory/disk on a home box).
const maxUploadBytes = 256 << 20

// Version é a versão do BINÁRIO, reportada por GET /api/v1/health e usada pelo
// front pra detectar deploy novo com a aba aberta (lib/version.ts + useVersionCheck).
//
// É var, não const, porque o Dockerfile injeta a tag do release via
// `-ldflags "-X autodj/adapters/driving/httpapi.Version=$APP_VERSION"` — a MESMA
// APP_VERSION que vira NEXT_PUBLIC_APP_VERSION no bundle. Servidor e bundle saem do
// mesmo build, então divergir significa que o servidor foi trocado embaixo da aba.
//
// O default "dev" é o que roda em build local sem ldflags (task serve, go run) e é o
// sinal pro front DESLIGAR a checagem: fora do Docker o bundle e o binário são
// compilados em momentos diferentes e divergiriam à toa, pedindo reload sem motivo.
var Version = "dev"

// Router wires HTTP routes to services.
type Router struct {
	lib       inbound.LibraryService
	play      inbound.PlaybackService
	content   inbound.ContentService
	mix       inbound.MixService
	sched     inbound.ScheduleService
	autopilot inbound.AutopilotService
	cartwall  inbound.CartwallService
	spotify   inbound.SpotifyService
	aidj      inbound.AIDJService
	rds       inbound.RDSService
	linein    inbound.LineInService
	auth      inbound.AuthService
	profile   inbound.ProfileService
	playlog   inbound.PlayLogService
	reports   inbound.ReportService
	requests  inbound.RequestService
	public    inbound.PublicSiteService
	social    inbound.SocialService
	staff     inbound.StaffScheduleService
	youtube   inbound.YouTubeService
	// stems: separação de vocal por IA (pré-render). Nil = ferramenta não
	// instalada nesta máquina; as rotas nem são registradas e a UI esconde a
	// função em vez de oferecer um clique que só pode falhar.
	stems inbound.StemService
	// spotifyReturn is the UI path the OAuth callback redirects to on success.
	spotifyReturn string
	// youtubeReturn is the UI path the YouTube OAuth callback redirects to.
	youtubeReturn string
	wsHandler     http.Handler
	// artwork extraída de arquivo local: cache + teto de processos. Sem isto o
	// endpoint roda um ffmpeg POR REQUISIÇÃO — ver artworkcache.go.
	artCache *artworkCache
}

// Option customizes the Router with optional collaborators the base constructor
// doesn't require, so existing call sites keep compiling unchanged.
type Option func(*Router)

// WithAutopilot enables the /autopilot status/toggle endpoints (Story 4.5).
// Without it those routes are simply not registered.
func WithAutopilot(a inbound.AutopilotService) Option {
	return func(rt *Router) { rt.autopilot = a }
}

// WithStems enables the AI vocal-separation endpoints. Sem ele (ferramenta não
// instalada) as rotas não existem — a UI descobre pelo status e esconde a função.
func WithStems(s inbound.StemService) Option {
	return func(rt *Router) { rt.stems = s }
}

// WithCartwall enables the /cartwall board endpoints (Story 5.4). Without it
// those routes are simply not registered.
func WithCartwall(c inbound.CartwallService) Option {
	return func(rt *Router) { rt.cartwall = c }
}

// WithSpotify enables the /spotify OAuth endpoints (Story 6.1). Without it those
// routes are simply not registered (graceful degradation when the integration is
// unconfigured). returnPath is where the callback redirects on success (defaults
// to "/" when empty).
func WithSpotify(s inbound.SpotifyService, returnPath string) Option {
	return func(rt *Router) {
		rt.spotify = s
		if returnPath == "" {
			returnPath = "/"
		}
		rt.spotifyReturn = returnPath
	}
}

// WithYouTube enables the /youtube OAuth + catalog endpoints (Story 6.5), a
// faithful mirror of WithSpotify. Without it those routes are simply not
// registered (graceful degradation when the integration is unconfigured — no
// client id/secret/API key). returnPath is where the callback redirects on
// success (defaults to "/" when empty).
func WithYouTube(s inbound.YouTubeService, returnPath string) Option {
	return func(rt *Router) {
		rt.youtube = s
		if returnPath == "" {
			returnPath = "/"
		}
		rt.youtubeReturn = returnPath
	}
}

// WithAIDJ enables the /aidj spoken-segue endpoints (Story 6.4). Without it those
// routes are simply not registered (graceful degradation when TTS is unconfigured).
func WithAIDJ(a inbound.AIDJService) Option {
	return func(rt *Router) { rt.aidj = a }
}

// WithRDS enables the /rds config/state/refresh endpoints (Story 7.1). Without it
// those routes are simply not registered (graceful degradation when the RDS
// sidecar is unconfigured).
func WithRDS(s inbound.RDSService) Option {
	return func(rt *Router) { rt.rds = s }
}

// WithLineIn enables the /linein live-capture endpoints (Story 7.2). Without it
// those routes are simply not registered (graceful degradation when no capture
// device/controller is configured).
func WithLineIn(s inbound.LineInService) Option {
	return func(rt *Router) { rt.linein = s }
}

// WithAuth enables the /auth login/me endpoints and the users-manage CRUD, and
// arms the RBAC middleware (Story 7.3). Without it those routes are not
// registered and requirePerm is a no-op pass-through — the home build stays open.
func WithAuth(a inbound.AuthService) Option {
	return func(rt *Router) { rt.auth = a }
}

// WithProfile enables the /profile/me layout endpoints (Story 7.4). It needs an
// AuthService wired too (the owner is resolved from the bearer token); without
// both, the routes are simply not registered.
func WithProfile(p inbound.ProfileService) Option {
	return func(rt *Router) { rt.profile = p }
}

// WithPlayLog enables the /playlog history and /playlog/top ranking endpoints
// (Story 7.5). Without it those routes are simply not registered.
func WithPlayLog(s inbound.PlayLogService) Option {
	return func(rt *Router) { rt.playlog = s }
}

// WithReports enables the /reports/ecad royalty-report (Story 8.1) and
// /reports/broadcast irradiation-proof (Story 8.2) endpoints. Without it neither
// route is registered.
func WithReports(s inbound.ReportService) Option {
	return func(rt *Router) { rt.reports = s }
}

// WithRequests enables the listener-request moderation endpoints (Story 8.3):
// POST/GET /requests and POST /requests/{id}/{approve,reject,played}. Without it
// none of the routes are registered.
func WithRequests(s inbound.RequestService) Option {
	return func(rt *Router) { rt.requests = s }
}

// WithPublicSite enables the read-only public snapshot endpoint (Story 8.4):
// GET /public/site (now-playing + upcoming + recent + daily info). It is public
// (no auth). Without it the route is not registered.
func WithPublicSite(s inbound.PublicSiteService) Option {
	return func(rt *Router) { rt.public = s }
}

// WithSocial enables the social-integrations endpoints (Story 8.5):
// GET /social/platforms, POST /social/announce, POST /social/post. Without it
// those routes are not registered.
func WithSocial(s inbound.SocialService) Option {
	return func(rt *Router) { rt.social = s }
}

// WithStaffSchedule enables the staff-schedule endpoints (Story 9.1):
// GET/POST /staff/shifts, DELETE /staff/shifts/{id}, GET /staff/onair. Without it
// those routes are not registered.
func WithStaffSchedule(s inbound.StaffScheduleService) Option {
	return func(rt *Router) { rt.staff = s }
}

// New builds the chi router. All REST routes are versioned under /api/v1.
// wsHandler serves the /ws live feed (may be nil). Optional collaborators (e.g.
// the autopilot) are supplied via Option.
func New(lib inbound.LibraryService, play inbound.PlaybackService, content inbound.ContentService, mix inbound.MixService, sched inbound.ScheduleService, wsHandler http.Handler, opts ...Option) http.Handler {
	rt := &Router{
		lib: lib, play: play, content: content, mix: mix, sched: sched,
		wsHandler: wsHandler, artCache: newArtworkCache(),
	}
	for _, o := range opts {
		o(rt)
	}
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(securityHeaders)
	r.Use(cors)

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/health", rt.health)
		r.Post("/library/scan", rt.scan)
		r.Get("/library/tracks", rt.tracks)
		r.Get("/library/categories", rt.categories)
		r.Get("/library/folders", rt.getFolders)
		r.Post("/library/folders", rt.postFolder)
		r.Delete("/library/folders/{name}", rt.deleteFolder)
		r.Put("/library/tracks/{id}/folders", rt.putTrackFolders)
		r.Post("/library/upload", rt.upload)
		r.Patch("/library/tracks/{id}", rt.updateTrack)
		r.Put("/library/tracks/{id}/markers", rt.setMarkers)
		r.Get("/library/tracks/{id}/waveform", rt.waveform)
		r.Get("/library/tracks/{id}/audio", rt.audio)
		r.Get("/library/tracks/{id}/artwork", rt.artwork)
		r.Delete("/library/tracks/{id}", rt.deleteTrack)
		r.Post("/library/clear", rt.clearLibrary)
		if rt.stems != nil {
			// Separação de vocal por IA (pré-render). POST pede, GET acompanha —
			// leva minutos, então NÃO é uma chamada síncrona que devolve o áudio.
			r.Post("/library/tracks/{id}/instrumental", rt.prepareInstrumental)
			r.Get("/library/tracks/{id}/instrumental", rt.instrumentalStatus)
			// lista o que está em andamento AGORA, para a grade inteira — dentro
			// do MESMO gate: sem a ferramenta a rota não existe, o cliente leva
			// 404 e simplesmente não desenha barra. Fora do gate ela responderia
			// `{}` em produção, fingindo um recurso que não está lá.
			r.Get("/library/instrumentals", rt.instrumentalsWorking)
		}
		r.Post("/playback/load", rt.load)
		r.Post("/playback/enqueue", rt.enqueue)
		r.Post("/playback/dequeue", rt.dequeue)
		r.Post("/playback/move", rt.move)
		r.Post("/playback/next", rt.next)
		r.Post("/playback/clear", rt.clear)
		r.Get("/playback/nowplaying", rt.nowPlaying)
		r.Get("/mix", rt.getMix)
		r.Put("/mix", rt.putMix)
		r.Get("/clocks", rt.getClocks)
		r.Put("/clocks/{id}", rt.putClock)
		r.Delete("/clocks/{id}", rt.deleteClock)
		r.Post("/clocks/{id}/generate", rt.generatePlaylist)
		r.Get("/grid", rt.getGrid)
		r.Put("/grid", rt.putGrid)
		r.Get("/events", rt.getEvents)
		r.Get("/events/due", rt.eventsDue)
		r.Put("/events/{id}", rt.putEvent)
		r.Delete("/events/{id}", rt.deleteEvent)
		r.Get("/ui/operational", rt.uiOperational)
		r.Get("/ui/webtabs", rt.uiWebTabs)
		r.Get("/ui/program", rt.uiProgram)
		r.Get("/ui/grade-load", rt.uiGradeLoad)
		if rt.autopilot != nil {
			r.Get("/autopilot", rt.getAutopilot)
			r.Post("/autopilot", rt.setAutopilot)
		}
		if rt.cartwall != nil {
			r.Get("/cartwall", rt.getCartwall)
			r.Put("/cartwall", rt.putCartwall)
			r.Post("/cartwall/buttons", rt.postCartButton)
			r.Delete("/cartwall/buttons/{id}", rt.deleteCartButton)
		}
		if rt.spotify != nil {
			r.Get("/spotify/status", rt.getSpotifyStatus)
			r.Post("/spotify/revalidate", rt.postSpotifyRevalidate)
			r.Get("/spotify/diagnose", rt.getSpotifyDiagnose)
			r.Post("/spotify/connect", rt.postSpotifyConnect)
			r.Get("/spotify/callback", rt.getSpotifyCallback)
			r.Post("/spotify/disconnect", rt.postSpotifyDisconnect)
			r.Get("/spotify/search", rt.getSpotifySearch)
			r.Post("/spotify/import", rt.postSpotifyImport)
			r.Get("/spotify/token", rt.getSpotifyToken)
		}
		if rt.youtube != nil {
			r.Get("/youtube/status", rt.getYouTubeStatus)
			r.Post("/youtube/connect", rt.postYouTubeConnect)
			r.Get("/youtube/callback", rt.getYouTubeCallback)
			r.Post("/youtube/disconnect", rt.postYouTubeDisconnect)
			r.Get("/youtube/search", rt.getYouTubeSearch)
			r.Post("/youtube/import", rt.postYouTubeImport)
			r.Post("/youtube/import-links", rt.postYouTubeImportLinks)
			r.Get("/youtube/token", rt.getYouTubeToken)
		}
		if rt.aidj != nil {
			r.Get("/aidj/config", rt.getAIDJConfig)
			r.Put("/aidj/config", rt.putAIDJConfig)
			r.Post("/aidj/voiceover", rt.postAIDJVoiceover)
		}
		if rt.rds != nil {
			r.Get("/rds/config", rt.getRDSConfig)
			r.Put("/rds/config", rt.putRDSConfig)
			r.Get("/rds/state", rt.getRDSState)
			r.Post("/rds/refresh", rt.postRDSRefresh)
		}
		if rt.linein != nil {
			r.Get("/linein", rt.getLineIn)
			r.Post("/linein/{id}/arm", rt.postLineInArm)
			r.Post("/linein/{id}/disarm", rt.postLineInDisarm)
			r.Put("/linein/{id}/levels", rt.putLineInLevels)
		}
		if rt.auth != nil {
			r.Post("/auth/login", rt.login)
			r.Get("/auth/me", rt.me)
			r.Route("/users", func(r chi.Router) {
				r.Use(rt.requirePerm(identity.PermUsersManage))
				r.Get("/", rt.listUsers)
				r.Post("/", rt.createUser)
				r.Patch("/{id}", rt.updateUser)
				r.Delete("/{id}", rt.deleteUser)
			})
		}
		if rt.profile != nil && rt.auth != nil {
			r.Get("/profile/me", rt.getProfile)
			r.Put("/profile/me", rt.putProfile)
		}
		if rt.playlog != nil {
			r.Get("/playlog", rt.getPlayLog)
			r.Get("/playlog/top", rt.getPlayLogTop)
		}
		if rt.requests != nil {
			r.Post("/requests", rt.postRequest)
			r.Get("/requests", rt.getRequests)
			r.Post("/requests/{id}/approve", rt.postRequestApprove)
			r.Post("/requests/{id}/reject", rt.postRequestReject)
			r.Post("/requests/{id}/played", rt.postRequestPlayed)
		}

		if rt.reports != nil {
			r.Get("/reports/ecad", rt.getECADReport)
			r.Get("/reports/broadcast", rt.getBroadcastProof)
		}
		if rt.public != nil {
			r.Get("/public/site", rt.getPublicSite)
		}
		if rt.social != nil {
			r.Get("/social/platforms", rt.getSocialPlatforms)
			r.Post("/social/announce", rt.postSocialAnnounce)
			r.Post("/social/post", rt.postSocialCustom)
		}
		if rt.staff != nil {
			r.Get("/staff/shifts", rt.getStaffShifts)
			r.Post("/staff/shifts", rt.postStaffShift)
			r.Delete("/staff/shifts/{id}", rt.deleteStaffShift)
			r.Get("/staff/onair", rt.getStaffOnAir)
		}
	})
	if rt.wsHandler != nil {
		r.Handle("/ws", rt.wsHandler)
	}
	return r
}

/*
securityHeaders põe as proteções de navegador que faltavam — a resposta não trazia
nenhuma.

O que está aqui e por quê:

  - `X-Content-Type-Options: nosniff` — impede o navegador de "adivinhar" o tipo de
    um arquivo enviado pelo operador e tratar um MP3 como HTML.
  - `X-Frame-Options: SAMEORIGIN` — ninguém embute o painel num iframe alheio para
    roubar cliques. Não é DENY porque o app desktop serve a UI por um proxy próprio.
  - `Referrer-Policy: strict-origin-when-cross-origin` — ⚠️ É EXATAMENTE O PADRÃO DO
    NAVEGADOR, E ESTÁ AQUI DE PROPÓSITO. Política mais restritiva (`no-referrer`,
    `same-origin`) QUEBRA O YOUTUBE: desde dez/2025 o embed exige que a página-mãe
    mande `Referer`, e sem ele o player estoura erro 153 e o vídeo não toca. Fixar o
    valor certo impede que alguém "endureça" isso depois e derrube o playout.
  - `Strict-Transport-Security` só quando a requisição chegou por HTTPS (atrás do
    proxy do Fly isso vem no `X-Forwarded-Proto`). Mandar HSTS no `http://localhost`
    do desenvolvimento prenderia o navegador a HTTPS num host que não o serve.

O que deliberadamente NÃO está:

  - **CSP** — o playout carrega o SDK do Spotify e o iframe do YouTube de domínios
    de terceiro, e uma política errada aqui não degrada: emudece a estação. Entra
    quando houver como testar os três motores contra ela.
  - **Permissions-Policy** — a tentação seria desligar recursos não usados, mas
    `autoplay` é o coração deste app. Uma linha distraída aqui deixa o operador sem
    som depois de um refresh.
*/
func securityHeaders(next http.Handler) http.Handler { return SecurityHeaders(next) }

// SecurityHeaders é o mesmo middleware, exportado porque o HTML estático é servido
// FORA do router — e é justamente na página que X-Frame-Options protege contra
// clickjacking. Aplicá-lo só na API deixaria de fora o alvo principal.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "SAMEORIGIN")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
			h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

/*
cors libera SÓ origens conhecidas. Era `*`, e isso é um buraco concreto num app
que roda na máquina de quem usa: qualquer site aberto noutra aba podia falar com
o AutoDJ em localhost e LER a resposta — biblioteca, fila, e até `POST
/library/clear`. Um servidor público sem autenticação já é alcançável por
qualquer um de qualquer jeito, mas `*` entrega o NAVEGADOR do operador como
procurador, e é isso que a restrição corta.

Quem precisa de origem cruzada, e por isso não dá pra simplesmente remover:

  - `task web:dev` — a página vem do Next em :3000 e a API responde em :8095
    (ver `apiBase()`); é o fluxo normal de desenvolvimento;
  - o app desktop NÃO precisa: o Wails serve a UI por um reverse-proxy, então
    para o webview é tudo mesma origem.

Requisição sem `Origin` (mesma origem, curl, servidor-a-servidor) não recebe
cabeçalho nenhum — não precisa.
*/
func cors(next http.Handler) http.Handler {
	extra := splitOrigins(os.Getenv("AUTODJ_CORS_ORIGINS"))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if allowed := corsAllowed(r, origin, extra); allowed {
			// Devolve a origem EXATA, nunca `*`: com `*` o navegador recusaria
			// credenciais e, pior, valeria para qualquer site.
			w.Header().Set("Access-Control-Allow-Origin", origin)
			// Resposta varia conforme a origem — sem isso, um cache intermediário
			// serviria a um site a permissão dada a outro.
			w.Header().Add("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,PATCH,DELETE,OPTIONS")
			// `X-AutoDJ-Session` entra AGORA, e só entra porque a origem passou a
			// ser restrita: é o cabeçalho que escolhe a estação. Com `*` liberá-lo
			// deixaria qualquer site escolher a estação da vítima — antes ele
			// faltava, o que na prática quebrava o modo `web:dev` com sessão.
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type,X-AutoDJ-Session")
			w.Header().Set("Access-Control-Max-Age", "600")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// corsAllowed: mesma origem, loopback (dev e desktop) ou lista explícita do
// operador. Qualquer outra coisa não recebe permissão e o navegador barra.
func corsAllowed(r *http.Request, origin string, extra []string) bool {
	if origin == "" {
		return false // sem Origin não há o que autorizar
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	if u.Host == r.Host {
		return true
	}
	if h := u.Hostname(); h == "localhost" || h == "127.0.0.1" || h == "::1" {
		return true
	}
	for _, o := range extra {
		if o == origin {
			return true
		}
	}
	return false
}

func splitOrigins(v string) []string {
	var out []string
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func (rt *Router) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "version": Version})
}

func (rt *Router) scan(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Root string `json:"root"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Root == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "root required"})
		return
	}
	n, err := rt.lib.ScanFolder(r.Context(), body.Root)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"scanned": n})
}

func (rt *Router) tracks(w http.ResponseWriter, r *http.Request) {
	ts, err := rt.lib.Tracks(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, toTrackDTOs(ts))
}

func (rt *Router) categories(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, toCategoryDTOs(rt.lib.Categories()))
}

// upload handles a multipart media upload: file part "file" + form fields
// category/title/artist/album. Creates a local (PCM) Track.
func (rt *Router) upload(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid multipart body"})
		return
	}
	file, hdr, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "file required"})
		return
	}
	defer file.Close()
	t, err := rt.lib.Upload(r.Context(), inbound.UploadInput{
		Filename: hdr.Filename,
		Content:  file,
		Category: r.FormValue("category"),
		Title:    r.FormValue("title"),
		Artist:   r.FormValue("artist"),
		Album:    r.FormValue("album"),
		// pastas do envio: repetir o campo `folder` no multipart (uma por valor),
		// que é como o form manda lista sem inventar codificação própria.
		Folders: r.MultipartForm.Value["folder"],
	})
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toTrackDTO(t))
}

// updateTrack edits a track's metadata (title/artist/album/category).
func (rt *Router) updateTrack(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var body struct {
		Title    string `json:"title"`
		Artist   string `json:"artist"`
		Album    string `json:"album"`
		Category string `json:"category"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	t, err := rt.lib.UpdateTrack(r.Context(), id, inbound.UpdateInput{
		Title: body.Title, Artist: body.Artist, Album: body.Album, Category: body.Category,
	})
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toTrackDTO(t))
}

// setMarkers replaces a track's cue markers. Body: {markers:[{kind,pos_ms,label}]}.
func (rt *Router) setMarkers(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var body struct {
		Markers []struct {
			Kind  string `json:"kind"`
			PosMs int64  `json:"pos_ms"`
			Label string `json:"label"`
		} `json:"markers"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	ms := make([]library.Marker, 0, len(body.Markers))
	for _, m := range body.Markers {
		ms = append(ms, library.Marker{Kind: library.MarkerKind(m.Kind), PosMs: m.PosMs, Label: m.Label})
	}
	t, err := rt.lib.SetMarkers(r.Context(), id, ms)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toTrackDTO(t))
}

// waveform serves the peaks + duration for the waveform editor.
// GET /library/tracks/{id}/waveform?bars=N (N default 120, clamped to [16,2000]).
func (rt *Router) waveform(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	bars := clampBars(r.URL.Query().Get("bars"))
	wf, err := rt.lib.Waveform(r.Context(), id, bars)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toWaveformDTO(wf))
}

// audio streams a track's raw audio bytes to the browser DJ deck / cartwall for
// Web Audio decoding. http.ServeFile handles Range (seek), Content-Type,
// Accept-Ranges and modtime for free. Only Mixable && PCMAvailable tracks are
// served — a Spotify (DRM) track returns 403 before disk is touched.
// GET /library/tracks/{id}/audio
func (rt *Router) audio(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	t, err := rt.lib.Audio(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	/* `?stem=instrumental` serve o arquivo SEM VOCAL, quando ele já foi preparado
	   pela separação de IA.

	   Um PARÂMETRO na mesma rota, e não uma rota nova, de propósito: o `<audio>`
	   do deck já sabe falar com esta URL (Range, seek, cache do Service Worker,
	   buffer anti-queda). Uma segunda rota teria de reconquistar tudo isso.

	   Ausente o preparo, cai no original em silêncio: é o que faz o modo VOZ
	   continuar funcionando (por cancelamento de centro) enquanto o instrumental
	   não existe — nunca um 404 no meio da música. */
	path := t.Path
	if r.URL.Query().Get("stem") == "instrumental" && t.HasInstrumental() {
		path = t.InstrumentalPath
	}
	http.ServeFile(w, r, path)
}

// prepareInstrumental enfileira a separação por IA de uma faixa.
// POST /library/tracks/{id}/instrumental → {status}
func (rt *Router) prepareInstrumental(w http.ResponseWriter, r *http.Request) {
	st, err := rt.stems.Prepare(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": st})
}

// instrumentalStatus devolve o andamento — a UI faz poll enquanto está `running`.
// GET /library/tracks/{id}/instrumental → {status, error}
func (rt *Router) instrumentalStatus(w http.ResponseWriter, r *http.Request) {
	st, msg := rt.stems.Status(r.Context(), chi.URLParam(r, "id"))
	writeJSON(w, http.StatusOK, map[string]string{"status": st, "error": msg})
}

// instrumentalsWorking lista o que está sendo preparado AGORA, por id. Uma
// requisição para a lista inteira, em vez de uma por linha.
// GET /library/instrumentals → {"<id>":"running", …}
func (rt *Router) instrumentalsWorking(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, rt.stems.Working(r.Context()))
}

// artwork serves a track's cover, uniform across providers (hexagonal): the same
// route answers for local, Spotify or any future source. A track carrying an
// external ArtworkURL (Spotify album image, …) redirects there; a local file is
// probed for an embedded picture (ffmpeg → stdout). No cover → 404, and the UI
// falls back to a placeholder tile. Purely presentational: never touches the
// two-engine mix/broadcast rules.
// GET /library/tracks/{id}/artwork
func (rt *Router) artwork(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	t, err := rt.lib.Artwork(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	if t.ArtworkURL != "" {
		http.Redirect(w, r, t.ArtworkURL, http.StatusFound)
		return
	}
	// YouTube has no local file to probe: when the stored thumbnail is missing
	// (older imports / raw-URI drops that never carried one) derive the public
	// i.ytimg.com cover from the video id — no API key, mirrors Spotify always
	// shipping an album image so the endpoint stays uniform across providers.
	// Empty/unknown id → fall through to 404 → placeholder.
	if vid := youtubeVideoID(t.YouTubeURI); vid != "" {
		http.Redirect(w, r, "https://i.ytimg.com/vi/"+vid+"/mqdefault.jpg", http.StatusFound)
		return
	}
	// Arquivo local: capa embutida (APIC/PIC), extraída via ffmpeg — mas NO
	// MÁXIMO UMA VEZ por arquivo (`artworkcache.go`). Antes era um processo por
	// requisição: 84 faixas × cada recarga da tela, e as SEM capa repetiam pra
	// sempre porque 404 não fica em cache no browser. Isso esgotou a tabela de
	// processos da máquina durante os testes.
	data, err := rt.artCache.Get(r.Context(), t.Path)
	if err != nil {
		// negativo também ganha Cache-Control: sem isso o browser volta a pedir a
		// capa inexistente a cada carga, e o custo nunca cai do lado do cliente.
		// 1h (não 1 dia) porque a tag pode ganhar capa depois.
		w.Header().Set("Cache-Control", "public, max-age=3600")
		writeErr(w, library.ErrNoArtwork)
		return
	}
	w.Header().Set("Content-Type", http.DetectContentType(data))
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write(data)
}

// youtubeVideoID extracts the 11-char video id from a stored youtube_uri.
// Imports have carried two shapes over time: the canonical "youtube:video:<id>"
// and a bare "<id>" (raw-URI drops). Playlists ("youtube:playlist:<id>") have no
// single-video thumbnail → "". Non-YouTube tracks have an empty YouTubeURI → "".
func youtubeVideoID(uri string) string {
	if uri == "" {
		return ""
	}
	if strings.HasPrefix(uri, "youtube:video:") {
		return strings.TrimPrefix(uri, "youtube:video:")
	}
	if !strings.Contains(uri, ":") {
		return uri // bare id
	}
	return "" // playlist / unknown scheme
}

// clampBars parses the bars query param, defaulting to 120 and clamping to
// [16,2000] so a client can't request an absurd payload.
func clampBars(s string) int {
	const def, min, max = 120, 16, 2000
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	if n < min {
		return min
	}
	if n > max {
		return max
	}
	return n
}

// deleteTrack removes a track and its media file.
func (rt *Router) deleteTrack(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := rt.lib.DeleteTrack(r.Context(), id); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// clearLibrary wipes the whole catalog (header "Limpar" → começar do zero).
// Idempotent: empty library is not an error.
func (rt *Router) clearLibrary(w http.ResponseWriter, r *http.Request) {
	if err := rt.lib.ClearLibrary(r.Context()); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (rt *Router) load(w http.ResponseWriter, r *http.Request) {
	n, err := rt.play.LoadFromLibrary(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"queued": n})
}

func (rt *Router) enqueue(w http.ResponseWriter, r *http.Request) {
	var body struct {
		TrackID string `json:"track_id"`
		Pos     int    `json:"pos"`
		PlayNow bool   `json:"play_now"`
		Origin  string `json:"origin"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.TrackID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "track_id required"})
		return
	}
	if err := rt.play.Enqueue(r.Context(), body.TrackID, body.Pos, body.PlayNow, body.Origin); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (rt *Router) dequeue(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Pos int `json:"pos"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "pos required"})
		return
	}
	if err := rt.play.Dequeue(r.Context(), body.Pos); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (rt *Router) move(w http.ResponseWriter, r *http.Request) {
	var body struct {
		From int `json:"from"`
		To   int `json:"to"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "from/to required"})
		return
	}
	if err := rt.play.Move(r.Context(), body.From, body.To); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (rt *Router) next(w http.ResponseWriter, r *http.Request) {
	if err := rt.play.Next(r.Context()); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (rt *Router) clear(w http.ResponseWriter, r *http.Request) {
	if err := rt.play.Clear(r.Context()); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (rt *Router) nowPlaying(w http.ResponseWriter, r *http.Request) {
	np, q, err := rt.play.NowPlaying(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, nowPlayingDTO(np, q))
}

// getMix returns the station-wide fine-mixing settings. The mixing.Settings
// JSON tags match content.MixConfig, so the frontend reuses one shape.
func (rt *Router) getMix(w http.ResponseWriter, r *http.Request) {
	s, err := rt.mix.Get(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, s)
}

// putMix validates and persists the fine-mixing settings, returning the stored
// value. A malformed body → 400; an out-of-range value → 422 (via writeErr).
func (rt *Router) putMix(w http.ResponseWriter, r *http.Request) {
	var s mixing.Settings
	if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	out, err := rt.mix.Save(r.Context(), s)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// getClocks lists the station's hour templates.
func (rt *Router) getClocks(w http.ResponseWriter, r *http.Request) {
	cs, err := rt.sched.Clocks(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if cs == nil {
		cs = []schedule.Clock{}
	}
	writeJSON(w, http.StatusOK, cs)
}

// putClock upserts a clock. The {id} path param is authoritative — it overrides
// any id in the body so PUT /clocks/{id} is idempotent. A malformed body → 400;
// an incoherent clock → 422 (via writeErr).
func (rt *Router) putClock(w http.ResponseWriter, r *http.Request) {
	var c schedule.Clock
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	c.ID = chi.URLParam(r, "id")
	out, err := rt.sched.SaveClock(r.Context(), c)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// deleteClock removes a clock by id. Unknown id → 404 (via writeErr).
func (rt *Router) deleteClock(w http.ResponseWriter, r *http.Request) {
	if err := rt.sched.DeleteClock(r.Context(), chi.URLParam(r, "id")); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// generatePlaylist materializes a clock into an ordered playlist of tracks
// (preview only — nothing is played). Unknown clock → 404; an empty catalog
// (nothing generated) → 422 (via writeErr).
func (rt *Router) generatePlaylist(w http.ResponseWriter, r *http.Request) {
	tracks, err := rt.sched.GeneratePlaylist(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	out := make([]trackDTO, 0, len(tracks))
	for i := range tracks {
		out = append(out, toTrackDTO(&tracks[i]))
	}
	writeJSON(w, http.StatusOK, out)
}

// getGrid returns the weekly programming grid (empty {cells:[]} when unset).
func (rt *Router) getGrid(w http.ResponseWriter, r *http.Request) {
	g, err := rt.sched.Grid(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if g.Cells == nil {
		g.Cells = []schedule.Cell{}
	}
	writeJSON(w, http.StatusOK, g)
}

// putGrid validates and persists the whole weekly grid, returning the stored
// value. A malformed body → 400; an incoherent grid (bad weekday/hour, duplicate
// cell, unknown clock) → 422 (via writeErr).
func (rt *Router) putGrid(w http.ResponseWriter, r *http.Request) {
	var g schedule.Grid
	if err := json.NewDecoder(r.Body).Decode(&g); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	out, err := rt.sched.SaveGrid(r.Context(), g)
	if err != nil {
		writeErr(w, err)
		return
	}
	if out.Cells == nil {
		out.Cells = []schedule.Cell{}
	}
	writeJSON(w, http.StatusOK, out)
}

// getEvents lists the station's scheduled events (empty [] when none).
func (rt *Router) getEvents(w http.ResponseWriter, r *http.Request) {
	es, err := rt.sched.Events(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if es == nil {
		es = []schedule.ScheduledEvent{}
	}
	writeJSON(w, http.StatusOK, es)
}

// putEvent upserts a scheduled event. The {id} path param is authoritative — it
// overrides any id in the body so PUT /events/{id} is idempotent. A malformed
// body → 400; an incoherent event → 422 (via writeErr).
func (rt *Router) putEvent(w http.ResponseWriter, r *http.Request) {
	var e schedule.ScheduledEvent
	if err := json.NewDecoder(r.Body).Decode(&e); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	e.ID = chi.URLParam(r, "id")
	out, err := rt.sched.SaveEvent(r.Context(), e)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// deleteEvent removes an event by id. Unknown id → 404 (via writeErr).
func (rt *Router) deleteEvent(w http.ResponseWriter, r *http.Request) {
	if err := rt.sched.DeleteEvent(r.Context(), chi.URLParam(r, "id")); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// eventsDue resolves the events due at ?at=<RFC3339> into the ordered media to
// inject (preemption preview — nothing is played). Missing/invalid at → 400.
func (rt *Router) eventsDue(w http.ResponseWriter, r *http.Request) {
	raw := r.URL.Query().Get("at")
	if raw == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "at (RFC3339) required"})
		return
	}
	at, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "at must be RFC3339"})
		return
	}
	tracks, err := rt.sched.NextInjections(r.Context(), at)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	out := make([]trackDTO, 0, len(tracks))
	for i := range tracks {
		out = append(out, toTrackDTO(&tracks[i]))
	}
	writeJSON(w, http.StatusOK, out)
}

func (rt *Router) uiOperational(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, rt.content.Operational(r.Context()))
}

func (rt *Router) uiWebTabs(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, rt.content.WebTabs(r.Context()))
}

func (rt *Router) uiProgram(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, rt.content.Program(r.Context()))
}

func (rt *Router) uiGradeLoad(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, rt.content.GradeLoad(r.Context()))
}

// getAutopilot reports the autopilot's live state (enabled/queueLen/lastTick).
func (rt *Router) getAutopilot(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, rt.autopilot.Status(r.Context()))
}

// setAutopilot toggles unattended supervision. Body {"enabled":bool}. Enabling
// ticks once immediately (the returned status reflects the post-tick state). A
// malformed body → 400; a tick failure surfaces as 500 with the status omitted.
func (rt *Router) setAutopilot(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	st, err := rt.autopilot.SetEnabled(r.Context(), body.Enabled)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, st)
}

// writeErr maps domain errors to HTTP status codes: not-found→404,
// hotkey-clash→409, bad-category/validation→422, everything else→500.
func writeErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, library.ErrTrackNotFound),
		errors.Is(err, library.ErrNoAudioPath),
		errors.Is(err, library.ErrNoArtwork),
		errors.Is(err, outbound.ErrClockNotFound),
		errors.Is(err, outbound.ErrEventNotFound),
		errors.Is(err, cartwall.ErrNotFound),
		errors.Is(err, spotify.ErrNoAccount),
		errors.Is(err, youtube.ErrNoAccount),
		errors.Is(err, outbound.ErrRequestNotFound),
		errors.Is(err, outbound.ErrShiftNotFound),
		errors.Is(err, inbound.ErrLineInNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
	case errors.Is(err, spotify.ErrStateMismatch),
		errors.Is(err, spotify.ErrAuthExpired),
		errors.Is(err, youtube.ErrStateMismatch),
		errors.Is(err, youtube.ErrAuthExpired):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	case errors.Is(err, youtube.ErrOAuthUnavailable):
		// catálogo pode estar ligado (API key) sem cliente OAuth: só conectar falha.
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
	case errors.Is(err, inbound.ErrUserNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
	case errors.Is(err, inbound.ErrBadCredentials),
		errors.Is(err, inbound.ErrBadToken):
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": err.Error()})
	case errors.Is(err, library.ErrTrackNotMixable),
		errors.Is(err, inbound.ErrForbidden):
		writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
	case errors.Is(err, inbound.ErrNameTaken):
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
	case errors.Is(err, aidj.ErrTTSUnavailable),
		errors.Is(err, inbound.ErrEncoderUnavailable):
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
	case errors.Is(err, cartwall.ErrHotkeyTaken),
		errors.Is(err, aidj.ErrDisabled),
		errors.Is(err, requests.ErrInvalidTransition),
		errors.Is(err, inbound.ErrNothingPlaying),
		errors.Is(err, staffschedule.ErrOverlap),
		errors.Is(err, rds.ErrDisabled):
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
	case errors.Is(err, social.ErrBadPlatform),
		errors.Is(err, social.ErrNoTrack),
		errors.Is(err, social.ErrEmptyText),
		errors.Is(err, rds.ErrBadPI),
		errors.Is(err, rds.ErrBadPS),
		errors.Is(err, rds.ErrBadPTY),
		errors.Is(err, rds.ErrBadRT),
		errors.Is(err, linein.ErrBadLevel),
		errors.Is(err, linein.ErrEmptyLabel),
		errors.Is(err, identity.ErrEmptyName),
		errors.Is(err, identity.ErrBadRole),
		errors.Is(err, inbound.ErrBadPeriod),
		errors.Is(err, inbound.ErrBadRange),
		errors.Is(err, inbound.ErrBadStatus),
		errors.Is(err, requests.ErrEmptyListener),
		errors.Is(err, requests.ErrEmptyRequest),
		errors.Is(err, requests.ErrEmptyCreatedAt),
		errors.Is(err, operatorprofile.ErrBadTheme),
		errors.Is(err, operatorprofile.ErrBadDensity),
		errors.Is(err, operatorprofile.ErrDupPanel),
		errors.Is(err, operatorprofile.ErrUnknownPanel),
		errors.Is(err, operatorprofile.ErrBadScreens),
		errors.Is(err, operatorprofile.ErrBadScreen),
		errors.Is(err, aidj.ErrBadVoice),
		errors.Is(err, aidj.ErrEmptyText),
		errors.Is(err, cartwall.ErrEmptyID),
		errors.Is(err, cartwall.ErrEmptyLabel),
		errors.Is(err, cartwall.ErrBadPage),
		errors.Is(err, cartwall.ErrNotMixable),
		errors.Is(err, library.ErrBadCategory),
		errors.Is(err, library.ErrMarkersNotPCM),
		errors.Is(err, library.ErrBadMarkerKind),
		errors.Is(err, library.ErrMarkerOutOfRange),
		errors.Is(err, library.ErrDuplicateMarker),
		errors.Is(err, library.ErrMarkerOrder),
		errors.Is(err, library.ErrWaveformNotPCM),
		errors.Is(err, mixing.ErrNegativeFade),
		errors.Is(err, mixing.ErrFadeTooLong),
		errors.Is(err, mixing.ErrVolPercent),
		errors.Is(err, schedule.ErrClockEmptyID),
		errors.Is(err, schedule.ErrClockNoName),
		errors.Is(err, schedule.ErrClockNoSlots),
		errors.Is(err, schedule.ErrBadSlotKind),
		errors.Is(err, schedule.ErrBadSlotCategory),
		errors.Is(err, schedule.ErrSlotCount),
		errors.Is(err, schedule.ErrKindCategoryMismatch),
		errors.Is(err, schedule.ErrBadHour),
		errors.Is(err, schedule.ErrBadWeekday),
		errors.Is(err, schedule.ErrDuplicateCell),
		errors.Is(err, schedule.ErrUnknownClock),
		errors.Is(err, schedule.ErrEmptyCategory),
		errors.Is(err, schedule.ErrEventEmptyID),
		errors.Is(err, schedule.ErrBadEventKind),
		errors.Is(err, schedule.ErrEventHour),
		errors.Is(err, schedule.ErrEventMinute),
		errors.Is(err, schedule.ErrEventNoDays),
		errors.Is(err, schedule.ErrEventBadDay),
		errors.Is(err, schedule.ErrEventPriority),
		errors.Is(err, staffschedule.ErrEmptyID),
		errors.Is(err, staffschedule.ErrEmptyAnnouncer),
		errors.Is(err, staffschedule.ErrBadWeekday),
		errors.Is(err, staffschedule.ErrBadRange):
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
