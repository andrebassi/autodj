// Package serverkit holds the AutoDJ backend wiring shared by the web server
// (cmd/server) and the desktop app (cmd/desktop). Build() assembles the hexagonal
// pieces — repo (Postgres / per-user SQLite / memory), engine (Liquidsoap / stub),
// every service, the boot seeds — and returns the ready HTTP handler (REST +
// WebSocket + static UI) plus a cleanup that tears the resources down. Both
// entrypoints stay thin: one serves it on a port, the other wraps it in a window.
// This is cmd-level (integration-tier) wiring — no domain logic lives here.
package serverkit

import (
	"context"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"autodj/adapters/driven/argon2id"
	"autodj/adapters/driven/audioanalyze"
	"autodj/adapters/driven/ffprobe"
	"autodj/adapters/driven/filestore"
	"autodj/adapters/driven/jwt"
	liblib "autodj/adapters/driven/library"
	lineinctrl "autodj/adapters/driven/linein"
	"autodj/adapters/driven/memory"
	"autodj/adapters/driven/postgres"
	rdsenc "autodj/adapters/driven/rds"
	"autodj/adapters/driven/socialstub"
	spotifyoauth "autodj/adapters/driven/spotify"
	"autodj/adapters/driven/sqlite"
	"autodj/adapters/driven/stemsep"
	"autodj/adapters/driven/stubengine"
	"autodj/adapters/driven/tts"
	"autodj/adapters/driven/waveform"
	youtubeoauth "autodj/adapters/driven/youtube"
	"autodj/adapters/driving/httpapi"
	"autodj/adapters/driving/ws"
	"autodj/app"
	"autodj/domain/aidj"
	domlinein "autodj/domain/linein"
	"autodj/ports/outbound"
)

// Build assembles the full backend and returns the HTTP handler (REST + WebSocket
// + static UI) ready to serve, plus a cleanup func that closes the DB and stops
// background workers (call it on shutdown, after cancelling ctx). Background loops
// (hub, autopilot, playlog recorder) are bound to ctx and stop when it cancels.
// buildTenant assembles ONE isolated backend stack (repos + engine + services +
// WS hub + background workers) bound to a single SQLite file (dbPath) and returns
// its API handler (REST + WebSocket, no static UI) plus a cleanup. Build() calls
// it once per browser fingerprint so each browser gets its own private station.
// dbPath: "memory" → ephemeral per-tenant repo; a file path → per-tenant SQLite;
// DATABASE_URL in env still forces shared Postgres (multi-tenant only for local
// SQLite/memory — the home use case).
// tenantID: o fingerprint já sanitizado (ou "default"). Isola o diretório de
// mídia junto com o banco — ver o bloco do media store abaixo.
// Every station — default OR real fingerprint — boots EMPTY. There is no demo
// seeding: the operator fills the catalog (scan folders, import Spotify/YouTube).
func buildTenant(ctx context.Context, tenantID, dbPath string) (http.Handler, func(), error) {
	var cleanups []func()
	cleanup := func() {
		for i := len(cleanups) - 1; i >= 0; i-- {
			cleanups[i]()
		}
	}

	port := envOr("PORT", "8095")

	// Repo precedence: DATABASE_URL → Postgres (remote/scale); else the local
	// default is per-user SQLite (1 file, no daemon — the home library survives
	// restarts); AUTODJ_DB_PATH=memory opts back into the ephemeral repo.
	var repo outbound.TrackRepo
	var mixRepo outbound.MixSettingsRepo
	var clockRepo outbound.ClockRepo
	var gridRepo outbound.GridRepo
	var eventRepo outbound.EventRepo
	var cartwallRepo outbound.CartwallRepo
	var spotifyRepo outbound.SpotifyAccountRepo
	var youtubeRepo outbound.YouTubeAccountRepo
	var userRepo outbound.UserRepo
	var profileRepo outbound.ProfileRepo
	var playLogRepo outbound.PlayLogRepo
	var requestRepo outbound.RequestRepo
	var staffRepo outbound.StaffScheduleRepo
	var folderRepo outbound.FolderCatalog // catálogo de pastas (só sqlite/memory)
	switch {
	case os.Getenv("DATABASE_URL") != "":
		pool, err := postgres.Connect(ctx, os.Getenv("DATABASE_URL"))
		if err != nil {
			return nil, cleanup, err
		}
		cleanups = append(cleanups, pool.Close)
		pg := postgres.New(pool)
		repo = pg
		mixRepo = pg.Mixing()
		clockRepo = pg.Clocks()
		gridRepo = pg.Grid()
		eventRepo = pg.Events()
		cartwallRepo = pg.Cartwall()
		spotifyRepo = pg.SpotifyAccount()
		youtubeRepo = pg.YouTubeAccount()
		userRepo = pg.Users()
		profileRepo = pg.Profiles()
		playLogRepo = pg.PlayLog()
		requestRepo = pg.Requests()
		staffRepo = pg.Staff()
		log.Println("repo: postgres")
	case dbPath == "memory" || dbPath == ":memory:":
		repo = memory.New()
		mixRepo = memory.NewMixRepo()
		clockRepo = memory.NewClockRepo()
		gridRepo = memory.NewGridRepo()
		eventRepo = memory.NewEventRepo()
		cartwallRepo = memory.NewCartwallRepo()
		spotifyRepo = memory.NewSpotifyAccountRepo()
		youtubeRepo = memory.NewYouTubeAccountRepo()
		userRepo = memory.NewUserRepo()
		profileRepo = memory.NewProfileRepo()
		playLogRepo = memory.NewPlayLogRepo()
		requestRepo = memory.NewRequestRepo()
		staffRepo = memory.NewStaffRepo()
		log.Println("repo: memory (ephemeral)")
	default:
		sq, closeFn, err := sqlite.Open(dbPath)
		if err != nil {
			return nil, cleanup, err
		}
		cleanups = append(cleanups, func() { _ = closeFn() })
		repo = sq
		folderRepo = sq.Folders()
		mixRepo = sq.Mixing()
		clockRepo = sq.Clocks()
		gridRepo = sq.Grid()
		eventRepo = sq.Events()
		cartwallRepo = sq.Cartwall()
		spotifyRepo = sq.SpotifyAccount()
		youtubeRepo = sq.YouTubeAccount()
		userRepo = sq.Users()
		profileRepo = sq.Profiles()
		playLogRepo = sq.PlayLog()
		requestRepo = sq.Requests()
		staffRepo = sq.Staff()
		log.Printf("repo: sqlite (%s)", dbPath)
	}

	// Engine: playout roda no browser (2 motores: Spotify SDK / áudio local).
	// O servidor só orquestra fila/now-playing via stub — sem bus de broadcast.
	// Liquidsoap/Icecast foram descontinuados (não transmitimos mais no servidor).
	var engine outbound.PlaybackEngine = stubengine.New()
	log.Println("engine: browser playout (fila/now-playing no servidor, sem broadcast)")

	// Media store for browser uploads (default ~/.autodj/media, overridable).
	// POR ESTAÇÃO (0.2.16): grava em `<media>/<tenant>/`, espelhando o banco por
	// sessão. Antes o diretório era global e o id do track é `sha1(path)` — duas
	// estações subindo `intro.mp3` sobrescreviam o arquivo uma da outra e caíam no
	// mesmo id. O tenant "default" (curl/testes/primeiro paint) segue na raiz, que
	// é o layout legado, então biblioteca antiga continua resolvendo pelo path
	// absoluto já gravado na linha do track — zero migração, zero id reescrito.
	mediaTenant := tenantID
	if mediaTenant == defaultTenant {
		mediaTenant = ""
	}
	media := filestore.NewFor(os.Getenv("AUTODJ_MEDIA_DIR"), mediaTenant)
	libSvc := app.NewLibraryService(repo, liblib.New(), ffprobe.New(), media)
	if folderRepo != nil {
		libSvc = libSvc.WithFolders(folderRepo)
	}
	libSvc.SetWaveformGenerator(waveform.New())
	playSvc := app.NewPlaybackService(repo, engine)
	// No demo content: every station boots empty (no sample catalog, no program
	// preview/folders/cartwall). A fresh session shows no demo tracks or rows.
	contentSvc := app.NewContentService().WithDemo(false)
	mixSvc := app.NewMixService(mixRepo)
	cartwallSvc := app.NewCartwallService(cartwallRepo, repo)
	schedSvc := app.NewScheduleService(clockRepo, gridRepo, repo, eventRepo)
	// GradeLoad histogram reflects the real weekly grid for today (Story 4.2).
	contentSvc.SetGradeSource(gridRepo, clockRepo, systemClock{})

	// Async audio analysis (BPM/key/waveform), out-of-process. Uses the CLI
	// analyzer when any tool is on PATH, else Noop (tracks stay un-analyzed).
	// Precompute on ingest keeps the playout hot-path free of analysis cost.
	var analyzer outbound.AudioAnalyzer
	if cli := audioanalyze.New(os.Getenv("AUTODJ_PEAKS_DIR")); cli.Available() {
		analyzer = cli
		log.Println("analyzer: cli (aubio/keyfinder/audiowaveform)")
	} else {
		analyzer = audioanalyze.Noop{}
		log.Println("analyzer: noop (no analysis tool on PATH) — install via nix develop")
	}
	analysisSvc := app.NewAnalysisService(repo, analyzer, 2)
	analysisSvc.Start(ctx)
	cleanups = append(cleanups, analysisSvc.Stop)
	libSvc.SetAnalysisEnqueuer(analysisSvc)

	// No first-run seeding. A clean install leaves the library, clocks, grid and
	// events empty — the operator builds the real station from scratch (scan
	// folders, import Spotify/YouTube, create clocks/grid/events).

	// Optional auto-scan on boot.
	if root := os.Getenv("AUTODJ_LIB_ROOT"); root != "" {
		if n, err := libSvc.ScanFolder(ctx, root); err != nil {
			log.Printf("boot scan failed: %v", err)
		} else {
			log.Printf("boot scan: %d tracks from %s", n, root)
			if q, err := playSvc.LoadFromLibrary(ctx); err == nil {
				log.Printf("boot queue: %d tracks", q)
			}
		}
	}

	// Backfill: enqueue existing un-analyzed PCM tracks so a library persisted
	// before analysis existed gets filled in without re-upload.
	if n, err := analysisSvc.Backfill(ctx); err == nil && n > 0 {
		log.Printf("analysis backfill: %d tracks queued", n)
	}

	hub := ws.NewHub(playSvc, 2*time.Second)
	go hub.Run(ctx)

	// Autopilot: unattended supervision (Story 4.5). É um AutoDJ — piloto
	// automático LIGADO por padrão (a razão de existir: roda sozinho do boot,
	// mantém a fila cheia, nunca fica mudo). AUTODJ_AUTOPILOT=0 desliga p/
	// operação 100% manual; qualquer outro valor (ou ausente) = ligado.
	autopilot := app.NewAutopilot(systemClock{}, schedSvc, playSvc, repo, engine, 0)
	// Faixa nova avisa o piloto NA HORA: sem isso, adicionar música com a fila
	// vazia deixava a estação muda por até 15s (o intervalo de supervisão), o que
	// do lado do operador é indistinguível de travamento.
	libSvc.SetCatalogWatcher(autopilot)
	if os.Getenv("AUTODJ_AUTOPILOT") != "0" {
		if _, err := autopilot.SetEnabled(ctx, true); err != nil {
			log.Printf("autopilot: initial tick: %v", err)
		}
		log.Println("autopilot: enabled (padrão AutoDJ; AUTODJ_AUTOPILOT=0 desliga)")
	} else {
		log.Println("autopilot: disabled (AUTODJ_AUTOPILOT=0)")
	}
	go autopilot.Run(ctx, 0)

	// Spotify OAuth (Story 6.1): wired only when a public client id is configured;
	// otherwise the routes stay 404 (graceful degradation, like Liquidsoap→stub).
	// PKCE public client — no secret in the binary.
	/* Separação de vocal por IA (Demucs via `demucs.cpp`) — OPT-IN e LOCAL.

	   `scripts/build-demucs.sh` instala binário + pesos; sem eles o `Available()`
	   dá false, as rotas não são registradas e a UI esconde a função. Mesma
	   degradação graciosa do analisador (aubio/keyfinder) e do Spotify sem client
	   id: recurso ausente some da tela, nunca vira clique que só pode falhar.

	   Prod NÃO tem isso instalado de propósito: a VM é `shared-cpu-1x` com 1 GB e
	   a separação leva ~1,4× o tempo real. É recurso de máquina do operador. */
	stemDir := filepath.Join(dataHome(), "instrumentals")
	/* DOIS separadores possíveis, e a precedência importa.

	   LOCAL primeiro: se o operador instalou o `demucs.cpp` na máquina dele, é de
	   graça e não depende de rede. A NUVEM entra quando não há binário local —
	   caso de produção, onde a separação foi revertida na 0.2.36 por custo de VM
	   (US$ 125/mês) e volta agora por GPU alugada por segundo (< US$ 0,10 pelo
	   acervo inteiro, medido).

	   Sem nenhum dos dois, `Available()` é false, as rotas nem são registradas e a
	   UI esconde a função — a degradação graciosa de sempre. */
	var sep outbound.StemSeparator = stemsep.New()
	cloud := stemsep.NewRunPod()
	// AUTODJ_STEMSEP=cloud força a nuvem mesmo com binário local instalado — é o
	// que permite trocar CPU lenta por GPU rápida na máquina do operador, e o que
	// torna o caminho de nuvem testável onde o local existe.
	forceCloud := os.Getenv("AUTODJ_STEMSEP") == "cloud"
	if cloud.Available() && (forceCloud || !sep.Available()) {
		sep = cloud
		log.Printf("stem: separação por GPU na nuvem (RunPod) habilitada")
	}
	stemSvc := app.NewStemService(repo, sep, stemDir)
	apiOpts := []httpapi.Option{httpapi.WithAutopilot(autopilot), httpapi.WithCartwall(cartwallSvc)}
	if stemSvc.Available() {
		stemSvc.Start(ctx)
		apiOpts = append(apiOpts, httpapi.WithStems(stemSvc))
		// preparo AUTOMÁTICO de faixa nova — só liga quando o operador marcar em
		// ⚙ Configurações (o serviço lê o interruptor a cada faixa que chega).
		libSvc.SetStemQueue(stemSvc, mixRepo)
		// e LIGAR a opção vale pro acervo inteiro, não só pra faixa nova.
		mixSvc.SetStemBatch(stemSvc)
		log.Printf("stem: separação por IA disponível (instrumentais em %s)", stemDir)
	}
	if clientID := os.Getenv("AUTODJ_SPOTIFY_CLIENT_ID"); clientID != "" {
		// 127.0.0.1 (não "localhost"): Spotify exige o IP de loopback explícito p/
		// apps novos e trata as duas strings como distintas — casa com o guia de setup.
		redirect := envOr("AUTODJ_SPOTIFY_REDIRECT", "http://127.0.0.1:"+port+"/api/v1/spotify/callback")
		oauth := spotifyoauth.New(clientID, redirect)
		catalog := spotifyoauth.NewCatalog()
		spotifySvc := app.NewSpotifyService(oauth, spotifyRepo, systemClock{}).WithCatalog(catalog, repo)
		// retorno pós-OAuth aponta pra tela onde a conta acabou de conectar
		// (Operacional → categoria Spotify), não a raiz crua.
		apiOpts = append(apiOpts, httpapi.WithSpotify(spotifySvc, envOr("AUTODJ_SPOTIFY_RETURN", "/?op=spotify")))
		log.Println("spotify: oauth + catalog enabled (AUTODJ_SPOTIFY_CLIENT_ID set)")
	} else {
		_ = spotifyRepo // repo built per branch; gateway off without a client id.
		log.Println("spotify: disabled (no AUTODJ_SPOTIFY_CLIENT_ID) — routes 404")
	}

	// YouTube OAuth + catalog (Story 6.5): the two halves are wired INDEPENDENTLY,
	// because they authenticate differently and serve different features.
	//
	//   Data API key   → catalog: search + import (paste-import needs no login)
	//   client id+secret → OAuth: linking a Google account (auth-code WITH secret,
	//                      not PKCE — the secret comes from env/pass, never source)
	//
	// Either half alone is useful: only the key gives a fully working tab without
	// any login; only OAuth links an account but has no catalog to read. Neither →
	// routes 404 (graceful degradation).
	ytClientID := os.Getenv("AUTODJ_YOUTUBE_CLIENT_ID")
	ytSecret := os.Getenv("AUTODJ_YOUTUBE_CLIENT_SECRET")
	ytAPIKey := os.Getenv("AUTODJ_YOUTUBE_API_KEY")
	ytOAuthOn := ytClientID != "" && ytSecret != ""
	// A aba YouTube passa a existir SEMPRE (v0.2.22): buscar, resolver link colado,
	// importar e tocar funcionam sem credencial nenhuma, via InnerTube — os mesmos
	// endpoints que o site do YouTube usa. Antes a API key era obrigatória só porque
	// o import dependia da Data API; deixou de depender.
	//
	// O que cada credencial ainda ACRESCENTA (nenhuma é requisito):
	//   API key → Data API na frente do import: filtro `status.embeddable` do Google
	//             e playlist paginada inteira (o caminho grátis devolve a janela).
	//   OAuth   → motor de busca "Minha conta" (search.list) e conta ligada.
	var oauth outbound.YouTubeOAuth
	if ytOAuthOn {
		redirect := envOr("AUTODJ_YOUTUBE_REDIRECT", "http://127.0.0.1:"+port+"/api/v1/youtube/callback")
		oauth = youtubeoauth.New(ytClientID, ytSecret, redirect)
	}
	youtubeSvc := app.NewYouTubeService(oauth, youtubeRepo, systemClock{})
	var dataAPI *youtubeoauth.Catalog
	if ytAPIKey != "" {
		dataAPI = youtubeoauth.NewCatalog(ytAPIKey)
		youtubeSvc = youtubeSvc.WithCatalog(dataAPI, repo)
	}
	// InnerTube é o catálogo SEMPRE presente. Com dataAPI != nil ele a usa como
	// preferência no import e como fallback da busca; com nil, resolve tudo sozinho.
	free := youtubeoauth.NewInnerTube(dataAPI)
	youtubeSvc = youtubeSvc.WithFreeCatalog(free)
	if dataAPI == nil {
		// sem key, o catálogo "oficial" do serviço passa a ser o próprio InnerTube,
		// senão import/playlist ficariam sem catálogo nenhum.
		youtubeSvc = youtubeSvc.WithCatalog(free, repo)
	}
	// retorno pós-OAuth aponta pra aba YouTube (Operacional → categoria YouTube).
	apiOpts = append(apiOpts, httpapi.WithYouTube(youtubeSvc, envOr("AUTODJ_YOUTUBE_RETURN", "/?op=youtube")))
	log.Printf("youtube: enabled (oauth=%v data-api=%v — sem credencial ainda funciona via InnerTube)", ytOAuthOn, dataAPI != nil)

	// AI DJ spoken segues (Story 6.4): pick a TTS gateway by env — ElevenLabs when
	// a cloud key is set, else Piper when its binary + a pt-BR model are present,
	// else nil (config panel still works; voiceover returns 503). The synthesized
	// voice is PCM, so it rides the local bus and bridges DRM Spotify tracks.
	var ttsGW outbound.TTSGateway
	switch {
	case os.Getenv("AUTODJ_ELEVENLABS_API_KEY") != "":
		ttsGW = tts.NewElevenLabs(os.Getenv("AUTODJ_ELEVENLABS_API_KEY"))
		log.Println("aidj: elevenlabs tts enabled (AUTODJ_ELEVENLABS_API_KEY set)")
	default:
		if model := os.Getenv("AUTODJ_PIPER_MODEL"); model != "" {
			p := tts.NewPiper(map[aidj.Voice]string{aidj.DefaultVoice: model})
			if p.Available() {
				ttsGW = p
				log.Println("aidj: piper tts enabled (AUTODJ_PIPER_MODEL set)")
			}
		}
	}
	if ttsGW == nil {
		log.Println("aidj: no tts gateway — config panel works, voiceover returns 503")
	}
	aidjSvc := app.NewAIDJService(repo, media, ttsGW)
	apiOpts = append(apiOpts, httpapi.WithAIDJ(aidjSvc))

	// RDS sidecar (Story 7.1): a FileEncoder writes the RDS state (PI/PS/PTY/RT)
	// to AUTODJ_RDS_FILE for a hardware/software RDS tool to poll; without the env
	// there's no encoder (config panel works, refresh returns 503). RDS carries
	// metadata only — no audio — so it never touches the two-engine invariant.
	var rdsEnc outbound.RDSEncoder
	if f := os.Getenv("AUTODJ_RDS_FILE"); f != "" {
		rdsEnc = rdsenc.NewFileEncoder(f)
		log.Println("rds: file encoder enabled (AUTODJ_RDS_FILE set)")
	} else {
		log.Println("rds: no encoder (no AUTODJ_RDS_FILE) — config panel works, refresh returns 503")
	}
	rdsSvc := app.NewRDSService(rdsEnc)
	apiOpts = append(apiOpts, httpapi.WithRDS(rdsSvc))

	// Line-in (Story 7.2): the operator's live-capture inputs (mic/aux). The
	// NoopController is the home-use stub — arm/disarm/levels work end-to-end
	// against in-memory state without a hardware bus wired to Liquidsoap. Seed
	// inputs come from AUTODJ_LINEIN ("id:Label" comma list) or default mic/aux.
	// Line-in is the legitimate live PCM source — always mixable, never DRM.
	lineInSeed := parseLineInSeed(os.Getenv("AUTODJ_LINEIN"))
	lineInSvc := app.NewLineInService(lineinctrl.NewNoopController(), lineInSeed...)
	apiOpts = append(apiOpts, httpapi.WithLineIn(lineInSvc))
	log.Printf("linein: %d input(s) configured (noop controller)", len(lineInSeed))

	// Auth / RBAC (Story 7.3): always wired — accounts live in the same repo as the
	// library (per-user SQLite / Postgres / memory). Passwords are argon2id; tokens
	// are HMAC JWT. AUTODJ_JWT_SECRET signs them (a dev default is used when unset,
	// with a loud warning — never ship without it). Token TTL from AUTODJ_JWT_TTL
	// (default 12h). On an empty repo SeedAdmin creates the first admin from
	// AUTODJ_ADMIN_USER / AUTODJ_ADMIN_PASSWORD so the operator can log in on day one.
	jwtSecret := []byte(os.Getenv("AUTODJ_JWT_SECRET"))
	if len(jwtSecret) == 0 {
		jwtSecret = []byte("autodj-dev-insecure-secret-change-me")
		log.Println("auth: AUTODJ_JWT_SECRET unset — using an INSECURE dev secret; set it before exposing the server")
	}
	jwtTTL := 12 * time.Hour
	if v := os.Getenv("AUTODJ_JWT_TTL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			jwtTTL = d
		} else {
			log.Printf("auth: bad AUTODJ_JWT_TTL %q — using %s", v, jwtTTL)
		}
	}
	authSvc := app.NewAuthService(userRepo, argon2id.New(), jwt.New(jwtSecret, jwtTTL))
	adminUser := envOr("AUTODJ_ADMIN_USER", "admin")
	adminPass := os.Getenv("AUTODJ_ADMIN_PASSWORD")
	if adminPass == "" {
		adminPass = "admin"
		log.Println("auth: AUTODJ_ADMIN_PASSWORD unset — seeding admin with default password 'admin'; change it after first login")
	}
	operator, err := authSvc.SeedAdmin(ctx, adminUser, adminPass)
	if err != nil {
		log.Printf("auth: seed admin failed: %v", err)
	} else if operator.ID != "" {
		log.Printf("auth: seeded admin %q", operator.Name)
	}
	// When the seed was a no-op (repo already had accounts) load the operator by
	// name so single-user mode has a real identity to resolve to.
	if operator.ID == "" {
		if u, gerr := userRepo.GetByName(ctx, adminUser); gerr == nil {
			operator = u
		}
	}

	// Single-user (home 1:1) mode (Story 9.4): default ON — the console opens with
	// no login, every token-less request resolving to the operator (the admin). Set
	// AUTODJ_SINGLE_USER=0 to arm the real multi-user login/RBAC instead.
	if envBool("AUTODJ_SINGLE_USER", true) && operator.ID != "" {
		apiOpts = append(apiOpts, httpapi.WithAuth(app.NewSingleUserAuth(authSvc, operator)))
		log.Printf("auth: single-user mode (no login) — operator %q; set AUTODJ_SINGLE_USER=0 for multi-user", operator.Name)
	} else {
		apiOpts = append(apiOpts, httpapi.WithAuth(authSvc))
		log.Println("auth: rbac enabled (argon2id + jwt)")
	}

	// Operator profiles (Story 7.4): each logged-in operator's player layout
	// (theme/density/panel order) lives in the same repo as their library. The
	// shell reads it at login; saving is gated to the owner (or an admin) by the
	// service. Wired alongside auth — the endpoints resolve the owner from the token.
	profileSvc := app.NewProfileService(profileRepo)
	apiOpts = append(apiOpts, httpapi.WithProfile(profileSvc))
	log.Println("profile: operator layout enabled")

	// PlayLog (Story 7.5): the append-only source of truth for what actually aired
	// (ECAD / broadcast proof / most-played). The recorder observes the live
	// now-playing feed and writes one entry per track change; the domain snapshots
	// broadcasted/ecad_relevant from the two-engine matrix, so a DRM Spotify play
	// lands with both false. History/Top serve the reporting endpoints.
	playLogSvc := app.NewPlayLogService(playLogRepo, systemClock{})
	apiOpts = append(apiOpts, httpapi.WithPlayLog(playLogSvc))
	reportSvc := app.NewReportService(playLogRepo, systemClock{})
	apiOpts = append(apiOpts, httpapi.WithReports(reportSvc))
	// Requests (Story 8.3): listener request / dedication moderation queue.
	requestSvc := app.NewRequestService(requestRepo, systemClock{})
	apiOpts = append(apiOpts, httpapi.WithRequests(requestSvc))

	// Staff schedule (Story 9.1): announcer roster (the human grid).
	staffSvc := app.NewStaffScheduleService(staffRepo, systemClock{})
	apiOpts = append(apiOpts, httpapi.WithStaffSchedule(staffSvc))
	// Public site (Story 8.4): read-only public snapshot (now/upcoming/recent + daily info).
	publicSvc := app.NewPublicSiteService(playSvc, playLogRepo, systemClock{})
	apiOpts = append(apiOpts, httpapi.WithPublicSite(publicSvc))
	// Social (Story 8.5): announce now-playing / free-text notes to WhatsApp/X/Facebook.
	// Home build wires the credential-free stub gateway; a real gateway is just
	// another adapter.
	socialSvc := app.NewSocialService(playSvc, socialstub.New(), systemClock{})
	apiOpts = append(apiOpts, httpapi.WithSocial(socialSvc))
	recorder := app.NewPlayLogRecorder(playLogSvc, repo, systemClock{})
	go func() {
		tick := time.NewTicker(2 * time.Second)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				if np, _, err := playSvc.NowPlaying(ctx); err == nil {
					recorder.Observe(ctx, np)
				}
			}
		}
	}()
	log.Println("playlog: recorder polling now-playing (2s)")

	api := httpapi.New(libSvc, playSvc, contentSvc, mixSvc, schedSvc, hub, apiOpts...)
	return api, cleanup, nil
}

// Build assembles the public HTTP handler. It fronts the per-tenant stacks with
// a fingerprint dispatcher: /api/ and /ws resolve the caller's browser session
// (X-AutoDJ-Session header, or ?sid= for the WebSocket) to its own isolated stack
// (lazily built, cached, one SQLite file per fingerprint under the sessions dir);
// the static UI is shared. cleanup tears every open tenant down. When DATABASE_URL
// is set the dispatcher still runs but every tenant shares the one Postgres — the
// per-browser SQLite isolation is a local-mode (home) feature.
func Build(ctx context.Context) (http.Handler, func(), error) {
	disp := newTenantMux(ctx)
	// staticUI serves the exported Next.js UI and falls back to the dispatcher for
	// non-file paths (so API/WS still route through per-tenant resolution).
	mux := http.NewServeMux()
	mux.Handle("/api/", disp)
	mux.Handle("/ws", disp)
	mux.Handle("/", staticUI(disp))
	return mux, disp.Close, nil
}

// StaticFS, when non-nil, is served as the UI instead of reading WEB_DIR from
// disk. The desktop build (Wails .app bundle) sets it to an embedded web/out so
// the app is self-contained and portable — the bundle's working directory is not
// the project root, so a disk path would not resolve. The server build leaves it
// nil and serves WEB_DIR from disk as before.
var StaticFS fs.FS

// staticUI serves the exported Next.js UI: from StaticFS if the desktop build
// set it, else from web/out on disk if present, else falls back to the API (dev
// runs the Next dev server separately).
//
// Os dois ramos passam por httpapi.StaticCacheControl: sem ele o FileServer manda
// só Last-Modified e o browser cacheia o index.html por heurística, continuando a
// apontar pro chunk antigo depois de um deploy (a UI nova fica invisível até o
// usuário dar hard-refresh). Ver adapters/driving/httpapi/staticcache.go.
func staticUI(api http.Handler) http.Handler {
	if StaticFS != nil {
		return httpapi.SecurityHeaders(httpapi.StaticCacheControl(http.FileServer(http.FS(StaticFS))))
	}
	dir := envOr("WEB_DIR", "web/out")
	if _, err := os.Stat(dir); err != nil {
		return api
	}
	return httpapi.SecurityHeaders(httpapi.StaticCacheControl(http.FileServer(http.Dir(dir))))
}

// systemClock is the production outbound.Clock: the real wall clock. Defined here
// (cmd wiring, integration-tier) so the domain/app stay time-injectable.
type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// envBool reads a boolean-ish env var. Unset → def. "0", "false", "no", "off"
// (case-insensitive) → false; anything else non-empty → true.
func envBool(k string, def bool) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(k)))
	if v == "" {
		return def
	}
	switch v {
	case "0", "false", "no", "off":
		return false
	default:
		return true
	}
}

// parseLineInSeed builds the operator's live-capture inputs from an
// "id:Label,id2:Label2" spec. An empty/blank spec yields the home default: a
// microphone and an auxiliary. Malformed entries (empty id) are skipped;
// duplicate ids collapse in the service. Inputs start disarmed at unity gain.
func parseLineInSeed(spec string) []domlinein.Input {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		mic, _ := domlinein.NewInput("mic0", "Microfone")
		aux, _ := domlinein.NewInput("aux0", "Auxiliar")
		return []domlinein.Input{mic, aux}
	}
	var out []domlinein.Input
	for _, part := range strings.Split(spec, ",") {
		id, label, ok := strings.Cut(strings.TrimSpace(part), ":")
		if !ok {
			label = id // no colon: the whole part is the id, label mirrors it
		}
		id, label = strings.TrimSpace(id), strings.TrimSpace(label)
		if id == "" {
			continue
		}
		if label == "" {
			label = id
		}
		in, err := domlinein.NewInput(id, label)
		if err != nil {
			continue
		}
		out = append(out, in)
	}
	return out
}

// dataHome é a raiz dos dados do AutoDJ (~/.autodj), a mesma que guarda o banco,
// as sessões e a mídia. Sem home disponível, cai no diretório corrente.
func dataHome() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "autodj-data"
	}
	return filepath.Join(home, ".autodj")
}

// defaultDBPath is the per-user SQLite file (~/.autodj/library.db). Falls back to
// a relative path if the home dir is unavailable.
func defaultDBPath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "autodj-library.db"
	}
	return filepath.Join(home, ".autodj", "library.db")
}
