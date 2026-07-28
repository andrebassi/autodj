// Package inbound declares the driving-side use-case interfaces the delivery
// adapters (HTTP, WebSocket, Wails) call into. Implementations live in the
// application services wired in cmd.
package inbound

import (
	"context"
	"errors"
	"io"
	"time"

	"autodj/domain/aidj"
	"autodj/domain/cartwall"
	"autodj/domain/content"
	"autodj/domain/identity"
	"autodj/domain/library"
	"autodj/domain/linein"
	"autodj/domain/mixing"
	"autodj/domain/operatorprofile"
	"autodj/domain/playback"
	"autodj/domain/playlog"
	"autodj/domain/publicsite"
	"autodj/domain/rds"
	"autodj/domain/reports"
	"autodj/domain/requests"
	"autodj/domain/schedule"
	"autodj/domain/social"
	"autodj/domain/spotify"
	"autodj/domain/staffschedule"
	"autodj/domain/youtube"
)

// ErrEncoderUnavailable means no RDS encoder is wired: the RDS config still
// reads/writes (so the panel works) but no state can be pushed (Story 7.1).
var ErrEncoderUnavailable = errors.New("rds: encoder not available")

// UploadInput carries a browser-uploaded media file plus operator-supplied
// metadata. Content is the file bytes; the service saves it, probes it, and
// builds a local Track.
type UploadInput struct {
	Filename string
	Content  io.Reader
	Category string
	Title    string // optional; defaults to probed title, else filename
	Artist   string // optional; overrides probed artist when set
	Album    string // optional; overrides probed album when set
	// Folders são as pastas do operador atribuídas no envio; valem pro lote inteiro.
	Folders  []string
}

// UpdateInput carries the editable metadata of a Track. Source/path/duration
// and the derived flags are never editable — only these four fields.
type UpdateInput struct {
	Title    string
	Artist   string
	Album    string
	Category string
}

// WaveformData is the waveform-editor payload for a PCM track: the track
// duration (to map peaks/markers to time) plus the normalized [0,1] peaks.
type WaveformData struct {
	DurationMs int64
	Peaks      []float64
}

// LibraryService covers catalog scanning, browsing and CRUD maintenance.
type LibraryService interface {
	// ScanFolder scans root, extracts metadata, and upserts local Tracks.
	// Returns how many tracks were added/updated.
	ScanFolder(ctx context.Context, root string) (int, error)
	// Tracks lists the whole catalog.
	Tracks(ctx context.Context) ([]*library.Track, error)
	// Upload saves an uploaded file, probes it, and creates a local Track.
	Upload(ctx context.Context, in UploadInput) (*library.Track, error)
	// UpdateTrack edits the metadata (title/artist/album/category) of an
	// existing track. Returns library.ErrBadCategory for an invalid category.
	UpdateTrack(ctx context.Context, id string, in UpdateInput) (*library.Track, error)
	// DeleteTrack removes a track from the catalog and its media file.
	DeleteTrack(ctx context.Context, id string) error
	// ClearLibrary wipes the whole catalog and best-effort deletes media files
	// (header "Limpar" → começar do zero). Idempotent.
	ClearLibrary(ctx context.Context) error
	// Categories returns the fixed set of valid media categories.
	Categories() []library.Category
	// Folders lista as pastas com contagem: automáticas da fonte (Spotify/YouTube),
	// "Sem pasta" e as do operador.
	Folders(ctx context.Context) ([]library.FolderCount, error)
	// CreateFolder registra uma pasta vazia. Idempotente.
	CreateFolder(ctx context.Context, name string) error
	// DeleteFolder tira a pasta do catálogo e das faixas. NUNCA apaga faixa.
	DeleteFolder(ctx context.Context, name string) error
	// SetTrackFolders troca as pastas de uma faixa. Streaming → ErrFolderStreaming.
	SetTrackFolders(ctx context.Context, id string, folders []string) (*library.Track, error)
	// SetMarkers replaces a track's cue markers. Returns library.ErrTrackNotFound
	// if absent, or a domain validation error (non-PCM, bad kind, out of range,
	// duplicate, order) when the markers are invalid.
	SetMarkers(ctx context.Context, id string, markers []library.Marker) (*library.Track, error)
	// Waveform returns bars peaks + duration for the waveform editor. Returns
	// library.ErrTrackNotFound if absent, library.ErrWaveformNotPCM for a DRM
	// (non-PCM) track, or ErrWaveformUnavailable when no generator is wired.
	Waveform(ctx context.Context, id string, bars int) (WaveformData, error)
	// Audio resolves a track's raw-audio file for the DJ deck / cartwall to
	// fetch and decode. Returns library.ErrTrackNotFound if absent,
	// library.ErrTrackNotMixable for a DRM (non-mixable) track, or
	// library.ErrNoAudioPath when a mixable track has no file path.
	Audio(ctx context.Context, id string) (*library.Track, error)
	// Artwork resolves a track's cover for the player/queue/history thumbnails,
	// uniform across providers. Returns the Track so delivery can redirect to
	// ArtworkURL (external cover) or extract the local file's embedded picture.
	// library.ErrTrackNotFound if absent, library.ErrNoArtwork when there is no
	// cover source at all.
	Artwork(ctx context.Context, id string) (*library.Track, error)
}

// PlaybackService drives the playout queue and player.
type PlaybackService interface {
	// LoadFromLibrary fills the playout queue from the catalog (walking
	// skeleton: all mixable local tracks, crossfaded).
	LoadFromLibrary(ctx context.Context) (int, error)
	// Enqueue inserts a catalog track into the playout queue at pos (pos<0
	// appends). playNow skips the current track so the item plays immediately.
	// origin is an informational provenance label (playlist/grid/event name);
	// "" for a bare enqueue.
	Enqueue(ctx context.Context, trackID string, pos int, playNow bool, origin string) error
	// Dequeue removes the pending item at queue index pos (as returned by
	// Queue/NowPlaying). pos 0 is the now-playing item and cannot be dequeued —
	// use Next to skip it. Out-of-range pos is a no-op error.
	Dequeue(ctx context.Context, pos int) error
	// Move reorders the running order: it moves the pending item at index from to
	// index to (both as returned by Queue/NowPlaying). Index 0 is the now-playing
	// item and is off-limits on either end — reorder only touches the pending
	// tail. Indices clamp into the pending range; from==to is a no-op.
	Move(ctx context.Context, from, to int) error
	// Next skips to the next track.
	Next(ctx context.Context) error
	// Clear empties the whole playout: it drops every pending item and stops the
	// current track, returning the player to a from-scratch state.
	Clear(ctx context.Context) error
	// NowPlaying returns the current track plus the upcoming queue.
	NowPlaying(ctx context.Context) (playback.Playable, []playback.QueueItem, error)
	// Queue returns the pending items.
	Queue(ctx context.Context) []playback.QueueItem
}

// MixService reads and updates the station's fine-mixing settings.
type MixService interface {
	// Get returns the current fine-mixing settings (defaults if never saved).
	Get(ctx context.Context) (mixing.Settings, error)
	// Save validates and persists the fine-mixing settings, returning the
	// stored value. Returns a mixing validation error (unchanged store) when
	// the settings are out of range.
	Save(ctx context.Context, s mixing.Settings) (mixing.Settings, error)
}

// CartwallService reads and updates the operator's cartwall board (Story 5.4).
type CartwallService interface {
	// Board returns the current cartwall (cartwall.Default() when never saved).
	Board(ctx context.Context) (cartwall.Board, error)
	// SaveBoard validates and persists the whole board, returning the stored
	// value. Each button is re-validated (label/page/hotkey uniqueness and the
	// two-engine mixable guard); an invalid board leaves the store untouched.
	SaveBoard(ctx context.Context, b cartwall.Board) (cartwall.Board, error)
	// AssignButton upserts a single button into the board and returns the whole
	// updated board. Returns cartwall.ErrHotkeyTaken (hotkey clash),
	// cartwall.ErrEmptyLabel/ErrBadPage/ErrEmptyID (malformed) or
	// cartwall.ErrNotMixable (non-mixable track) without touching the store.
	AssignButton(ctx context.Context, btn cartwall.CartButton) (cartwall.Board, error)
	// ClearButton removes a button by id. Returns cartwall.ErrNotFound when the
	// id is unknown.
	ClearButton(ctx context.Context, id string) error
}

// SpotifyStatus is the connection state the panel renders: whether an account is
// linked and whether it is Premium (Web Playback SDK eligible). Tags JSON camelCase.
type SpotifyStatus struct {
	Connected bool `json:"connected"`
	Premium   bool `json:"premium"`
}

// SpotifyStep é um degrau do diagnóstico: o que foi verificado, se passou e o que
// dizer ao operador. `Hint` só vem preenchido quando há ação a tomar.
type SpotifyStep struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
	Hint   string `json:"hint,omitempty"`
}

// SpotifyDiagnosis é o resultado da cadeia de verificação, na ordem em que ela
// acontece de verdade — o primeiro degrau que falha é a causa, e os seguintes são
// consequência. Nasceu de uma investigação de horas em que o Spotify aceitava tudo
// (connect OK, device registrado, play 204) e não emitia som: o único sinal honesto
// era um 403 no perfil, visível apenas do servidor.
type SpotifyDiagnosis struct {
	Steps []SpotifyStep `json:"steps"`
	// OK é true quando nenhum degrau falhou.
	OK bool `json:"ok"`
	// Verdict resume em uma frase o que fazer (vazio quando está tudo certo).
	Verdict string `json:"verdict,omitempty"`
}

// SpotifyService owns the OAuth session that links the operator to Spotify
// (Story 6.1). It only handles authentication — begin the PKCE flow, complete
// the callback, report status, unlink, and hand out a fresh access token
// (auto-refreshing). It never creates a Track: the two-engine split is enforced
// downstream when 6.2 imports Spotify metadata as non-mixable tracks.
type SpotifyService interface {
	// Begin starts a PKCE OAuth flow: generates state + verifier, stores the
	// pending auth, and returns the accounts.spotify.com authorize URL.
	Begin(ctx context.Context) (authURL string, err error)
	// Complete finishes the callback: validates state (spotify.ErrStateMismatch
	// on clash, spotify.ErrAuthExpired past TTL), exchanges the code for tokens
	// and persists the account.
	Complete(ctx context.Context, state, code string) error
	// Status reports whether an account is connected and Premium.
	Status(ctx context.Context) (SpotifyStatus, error)
	// Revalidate força a renovação do token e devolve o estado REAL da conta.
	// Chamado quando o playout trava (device aceita comandos e não toca), pra
	// distinguir "token recusado" de "problema de outra natureza".
	Revalidate(ctx context.Context) (SpotifyStatus, error)
	// Diagnose roda a cadeia de verificação e devolve um passo a passo legível.
	// Só leitura — nunca inicia reprodução.
	Diagnose(ctx context.Context) (SpotifyDiagnosis, error)
	// Disconnect unlinks the account (idempotent — no error when already absent).
	Disconnect(ctx context.Context) error
	// AccessToken returns a valid access token, refreshing transparently when the
	// stored one is near expiry. Returns spotify.ErrNoAccount when unlinked.
	AccessToken(ctx context.Context) (string, error)
	// Search queries the Spotify catalog for tracks or playlists (kind =
	// spotify.KindTrack | spotify.KindPlaylist). Returns spotify.ErrNoAccount when
	// no account is linked, ErrSpotifyImportUnavailable when the catalog gateway
	// is not wired.
	Search(ctx context.Context, query, kind string) ([]spotify.CatalogItem, error)
	// ImportTracks imports track URIs into the catalog as non-mixable Spotify
	// tracks (metadata only — no audio), returning the created tracks.
	ImportTracks(ctx context.Context, uris []string) ([]library.Track, error)
	// ImportPlaylist imports every track of a playlist as non-mixable Spotify
	// tracks, returning the created tracks.
	ImportPlaylist(ctx context.Context, playlistID string) ([]library.Track, error)
}

// YouTubeStatus is the connection state the panel renders: whether a Google/
// YouTube account is linked, the best-effort Premium flag (chip parity only —
// playout never depends on it) and whether the free (InnerTube) search backend is
// wired, which is what lets the panel offer the search switch. Tags JSON
// camelCase.
type YouTubeStatus struct {
	Connected  bool `json:"connected"`
	Premium    bool `json:"premium"`
	FreeSearch bool `json:"freeSearch"`
	// OAuth indica se o servidor TEM client id/secret — sem eles o motor de busca
	// "Minha conta" é impossível (connect → 503) e a UI o esconde.
	OAuth bool `json:"oauth"`
}

// YouTubeService owns the OAuth session that links the operator to their Google/
// YouTube account (Story 6.5) — a faithful mirror of SpotifyService. It gates
// search/import/playout behind a mandatory Google login (auth-code flow WITH
// client secret, not PKCE). It never creates a Track directly except via import,
// where the two-engine split forces SourceYouTube ⇒ non-mixable.
type YouTubeService interface {
	// Begin starts an OAuth auth-code flow: generates state, stores the pending
	// auth, and returns the accounts.google.com authorize URL.
	Begin(ctx context.Context) (authURL string, err error)
	// Complete finishes the callback: validates state (youtube.ErrStateMismatch on
	// clash, youtube.ErrAuthExpired past TTL), exchanges the code for tokens and
	// persists the account.
	Complete(ctx context.Context, state, code string) error
	// Status reports whether an account is connected and its best-effort Premium.
	Status(ctx context.Context) (YouTubeStatus, error)
	// Disconnect unlinks the account (idempotent — no error when already absent).
	Disconnect(ctx context.Context) error
	// AccessToken returns a valid access token, refreshing transparently when the
	// stored one is near expiry. Returns youtube.ErrNoAccount when unlinked.
	AccessToken(ctx context.Context) (string, error)
	// Search queries the YouTube catalog for videos or playlists (kind =
	// youtube.KindVideo | youtube.KindPlaylist) through the chosen backend
	// (youtube.BackendFree | youtube.BackendAPI; unknown/"" → free).
	// BackendFree needs NO account and spends no quota; BackendAPI returns
	// youtube.ErrNoAccount when no account is linked. Either one returns
	// youtube.ErrCatalogUnavailable when its gateway is off.
	Search(ctx context.Context, query, kind, backend string) ([]youtube.CatalogItem, error)
	// ImportTracks imports video URIs/ids into the catalog as non-mixable YouTube
	// tracks (metadata only — no audio), returning the created tracks.
	ImportTracks(ctx context.Context, uris []string) ([]library.Track, error)
	// ImportPlaylist imports every video of a playlist as non-mixable YouTube
	// tracks, returning the created tracks.
	ImportPlaylist(ctx context.Context, playlistID string) ([]library.Track, error)
	// ImportLinks imports videos from free-form pasted text (links separated by
	// ";", newlines or commas). Needs NO linked account — resolving ids costs 1
	// Data API unit per batch. Text with no recognizable link →
	// youtube.ErrNoLinks; gateway off → youtube.ErrCatalogUnavailable.
	ImportLinks(ctx context.Context, raw string) ([]library.Track, error)
}

// AIDJConfig is the AI DJ's operator-facing settings: whether spoken segues are
// on and which voice narrates. Tags JSON camelCase for the panel.
type AIDJConfig struct {
	Enabled bool       `json:"enabled"`
	Voice   aidj.Voice `json:"voice"`
}

// AIDJService is the spoken auto-DJ (Story 6.4). It renders a segue line for the
// next track, synthesizes it (source=tts), and returns it as a mixable local
// Track — the *voice* that bridges a DRM Spotify track on the local bus while the
// Spotify SDK only fades its own volume (engines cooperate, DRM audio never
// crosses). Synthesis is cached by text+voice hash so repeated lines are free.
type AIDJService interface {
	// Config reports whether spoken segues are enabled and the chosen voice.
	Config(ctx context.Context) AIDJConfig
	// SetConfig updates the enabled flag and voice, returning the stored config.
	// Returns ErrBadVoice when the voice is unknown (aidj.ValidVoice guards it).
	SetConfig(ctx context.Context, cfg AIDJConfig) (AIDJConfig, error)
	// Voiceover renders the segue line for the track forTrackID, synthesizes it
	// (or reads cache) and returns a source=tts mixable Track ready to enqueue
	// ahead of the target. Returns library.ErrTrackNotFound when the target is
	// unknown, ErrTTSUnavailable when no gateway is wired, or ErrAIDJDisabled when
	// spoken segues are off.
	Voiceover(ctx context.Context, forTrackID string) (library.Track, error)
}

// RDSConfig mirrors the station-fixed RDS identity (rds.Config) for the operator
// panel: whether RDS output is on plus the PI/PS/PTY codes. Tags JSON so the panel
// round-trips it verbatim.
type RDSConfig struct {
	Enabled bool   `json:"enabled"`
	PI      uint16 `json:"pi"`
	PS      string `json:"ps"`
	PTY     uint8  `json:"pty"`
}

// RDSService drives the Radio Data System sidecar (Story 7.1): it holds the fixed
// station identity and, when enabled, derives the dynamic RadioText from now-playing
// metadata and pushes the full state to the encoder. RDS carries *metadata only* —
// no audio — so a DRM Spotify track may name RadioText without breaching the
// two-engine invariant (no PCM crosses).
type RDSService interface {
	// Config reports the current station-fixed RDS identity.
	Config(ctx context.Context) RDSConfig
	// SetConfig validates (via the rds domain) and stores the identity, returning
	// the stored value. Returns an rds validation error (unchanged store) when invalid.
	SetConfig(ctx context.Context, cfg RDSConfig) (RDSConfig, error)
	// UpdateNowPlaying rebuilds the RDS state from the given metadata and pushes it
	// to the encoder, returning the pushed state. Returns rds.ErrDisabled when RDS
	// is off, an rds validation error for a bad identity, or ErrEncoderUnavailable
	// when no encoder is wired.
	UpdateNowPlaying(ctx context.Context, title, artist string) (rds.State, error)
	// Current returns the last state pushed (zero State before the first push).
	Current(ctx context.Context) rds.State
}

// ErrLineInNotFound means no line-in input matches the given id.
var ErrLineInNotFound = errors.New("linein: input not found")

// LineInService arms live PCM captures (mic/aux/soundcard) onto the local playout
// bus (Story 7.2). Line-in is the legitimate *live* PCM source — always mixable,
// never DRM — so it ducks the music underneath a voice without breaching the
// two-engine invariant (Spotify stays unmixable; line-in is real PCM).
type LineInService interface {
	// List reports every configured input with its current arm/level state.
	List(ctx context.Context) []linein.Input
	// Arm arms the input id onto the bus, commanding the controller, and returns
	// the updated input. Returns ErrLineInNotFound for an unknown id.
	Arm(ctx context.Context, id string) (linein.Input, error)
	// Disarm removes the input id from the bus (idempotent). Returns
	// ErrLineInNotFound for an unknown id.
	Disarm(ctx context.Context, id string) (linein.Input, error)
	// SetGain sets the pass-through gain [0,1] of input id, pushing to the
	// controller when armed. Returns linein.ErrBadLevel when out of range or
	// ErrLineInNotFound for an unknown id.
	SetGain(ctx context.Context, id string, v float64) (linein.Input, error)
	// SetDuck sets the music-duck level [0,1] of input id, pushing to the
	// controller when armed. Returns linein.ErrBadLevel when out of range or
	// ErrLineInNotFound for an unknown id.
	SetDuck(ctx context.Context, id string, v float64) (linein.Input, error)
}

// Auth sentinels (Story 7.3). Kept in inbound, next to the other service errors,
// so httpapi.writeErr maps them via the already-imported inbound package without
// a dependency on the app layer.
var (
	// ErrBadCredentials means the name is unknown or the password did not verify.
	// Deliberately opaque — callers must not tell the two apart.
	ErrBadCredentials = errors.New("auth: invalid credentials")
	// ErrForbidden means the authenticated user's role lacks the permission.
	ErrForbidden = errors.New("auth: forbidden")
	// ErrBadToken means a token is missing, malformed, tampered or expired.
	ErrBadToken = errors.New("auth: invalid token")
	// ErrUserNotFound means no user matches the given id.
	ErrUserNotFound = errors.New("auth: user not found")
	// ErrNameTaken means a user with that name already exists.
	ErrNameTaken = errors.New("auth: name already taken")
)

// AuthService authenticates users and gates access by the fixed role→permission
// matrix (Story 7.3). Login exchanges name+password for a short-lived token;
// Authorize validates a token and checks one permission. User CRUD is itself
// gated: every mutation requires an actor holding identity.PermUsersManage.
// Passwords never cross this interface in the clear beyond Login's input — they
// are hashed by the PasswordHasher port and stored only as opaque hashes.
type AuthService interface {
	// Login verifies name+password and issues a token. Returns ErrBadCredentials
	// for an unknown name or a wrong password (indistinguishable on purpose).
	Login(ctx context.Context, name, password string) (string, identity.User, error)
	// Authorize parses the token and checks perm against the user's role. Returns
	// ErrBadToken for a missing/invalid token or ErrForbidden when the role lacks
	// the permission. Returns the authenticated user on success.
	Authorize(ctx context.Context, token string, perm identity.Permission) (identity.User, error)
	// Me parses the token and returns the user without a permission check. Returns
	// ErrBadToken for a missing/invalid token.
	Me(ctx context.Context, token string) (identity.User, error)
	// ListUsers returns every account (hashes stripped by the caller/DTO). Requires
	// the actor to hold users.manage. Returns ErrForbidden otherwise.
	ListUsers(ctx context.Context, actor identity.User) ([]identity.User, error)
	// CreateUser validates and stores a new account with the given plaintext
	// password (hashed via the port). Requires users.manage. Returns ErrForbidden,
	// an identity validation error, or ErrNameTaken.
	CreateUser(ctx context.Context, actor identity.User, name string, role identity.Role, password string) (identity.User, error)
	// UpdateUser changes a user's role and, when password is non-empty, its
	// password. Requires users.manage. Returns ErrForbidden, ErrUserNotFound or an
	// identity validation error.
	UpdateUser(ctx context.Context, actor identity.User, id string, role identity.Role, password string) (identity.User, error)
	// DeleteUser removes a user by id. Requires users.manage. Returns ErrForbidden
	// or ErrUserNotFound.
	DeleteUser(ctx context.Context, actor identity.User, id string) error
}

// ProfileService reads and saves each operator's player layout (Story 7.4). A
// profile belongs to a user (from RBAC, Story 7.3): the shell reads it at login
// and applies theme/density/panel order. Saving is gated — only the owner or an
// admin may write a given user's layout — so no operator can rewrite another's.
type ProfileService interface {
	// Get returns userID's saved layout, or operatorprofile.Default(userID) when
	// none was ever saved (graceful default, never an error for absence).
	Get(ctx context.Context, userID string) (operatorprofile.Profile, error)
	// Save validates (via the domain) and persists a profile. The actor must be the
	// profile's owner or hold users.manage; otherwise ErrForbidden. Returns an
	// operatorprofile validation error (unchanged store) when the layout is invalid.
	Save(ctx context.Context, actor identity.User, p operatorprofile.Profile) (operatorprofile.Profile, error)
}

// ErrBadPeriod is returned by PlayLogService.Top for an unknown ranking window
// (anything other than "dia", "semana" or "mes").
var ErrBadPeriod = errors.New("playlog: period must be dia, semana or mes")

// PlayLogService records what played and serves the history and "most played"
// ranking (Story 7.5). Record is called at play time (append-only, immutable);
// History and Top drive the reports/UI. The record is the single source of
// truth for ECAD and the irradiation proof, so Spotify plays land in it with
// Broadcasted/ECADRelevant false and reports filter on those.
type PlayLogService interface {
	// Record snapshots a track that just played at playedAt into the append-only
	// log. Idempotent per (track, instant). Returns playlog.ErrBadEntry for a
	// track with no id or a zero playedAt.
	Record(ctx context.Context, t library.Track, playedAt time.Time) error
	// History returns the entries played in [from, to), oldest first.
	History(ctx context.Context, from, to time.Time) ([]playlog.Entry, error)
	// Top returns the n most played tracks over the named rolling window ending
	// now: "dia" (24h), "semana" (7d) or "mes" (30d). Returns ErrBadPeriod for
	// any other period.
	Top(ctx context.Context, period string, n int) ([]playlog.TopItem, error)
}

// ErrBadRange is returned by ReportService when the requested window is empty or
// inverted (from is not strictly before to).
var ErrBadRange = errors.New("reports: from must be strictly before to")

// ReportService derives read-only royalty/execution reports from the play log
// (Story 8.1). It owns no state: a report is a pure projection over the
// immutable log, so regenerating yields the same result. The ECAD report only
// aggregates ECAD-relevant plays, so DRM Spotify never appears.
type ReportService interface {
	// ECAD builds the ECAD execution report over [from, to): the ECAD-relevant
	// plays aggregated by track (plays + total aired duration), ordered by plays
	// descending. Returns ErrBadRange when from is not strictly before to.
	ECAD(ctx context.Context, from, to time.Time) (reports.ECADReport, error)

	// BroadcastProof builds the chronological broadcast proof over [from, to): every
	// aired play (Broadcasted=true), one line per play, ordered by play time — the
	// auditable evidence of what actually went to air. Returns ErrBadRange when from
	// is not strictly before to.
	BroadcastProof(ctx context.Context, from, to time.Time) (reports.BroadcastProof, error)
}

// ErrEmptyListener / ErrEmptyRequest are returned by RequestService.Submit when
// the listener name or song hint is blank (surfaced from the domain). ErrBadStatus
// is returned by RequestService.List for an unknown status filter.
var (
	ErrEmptyListener = requests.ErrEmptyListener
	ErrEmptyRequest  = requests.ErrEmptyRequest
	ErrBadStatus     = errors.New("requests: status must be pending, approved, rejected or played")
)

// RequestService owns the listener-request moderation queue (Story 8.3). Unlike
// the report services it has state: a Request is submitted pending and moderated
// through the domain transition matrix. The service generates ids from an
// injected Clock and persists via RequestRepo.
type RequestService interface {
	// Submit records a new pending request. listener and songHint are required
	// (ErrEmptyListener/ErrEmptyRequest); dedication is optional. The id and
	// CreatedAt come from the injected Clock.
	Submit(ctx context.Context, listener, songHint, dedication string) (requests.Request, error)
	// List returns requests ordered by id. status="" returns all; a valid status
	// filters; an unknown status returns ErrBadStatus.
	List(ctx context.Context, status string) ([]requests.Request, error)
	// Approve/Reject/MarkPlayed moderate the request with id, returning the updated
	// value. Unknown id → outbound.ErrRequestNotFound; forbidden transition →
	// requests.ErrInvalidTransition.
	Approve(ctx context.Context, id string) (requests.Request, error)
	Reject(ctx context.Context, id string) (requests.Request, error)
	MarkPlayed(ctx context.Context, id string) (requests.Request, error)
}

// PublicSiteService builds the read-only public snapshot — now-playing, the
// upcoming queue and the recent history plus the daily fact sheet (Story 8.4).
// It owns no state: the snapshot is a pure projection over the live playout and
// the append-only play log, regenerated per request. Unlike the ECAD/proof
// reports it lists what actually played (studio Spotify included).
type PublicSiteService interface {
	// Snapshot returns the current public view: now-playing + upcoming queue +
	// recent history (last 24h, newest first) + the daily info for now.
	Snapshot(ctx context.Context) (publicsite.Snapshot, error)
}

// ErrNothingPlaying is returned by SocialService.AnnounceNowPlaying when there is
// no track on the air to announce.
var ErrNothingPlaying = errors.New("social: nothing is playing")

// SocialService composes and publishes social posts (Story 8.5): a one-click
// "now playing" announcement across the selected platforms, or a free-text note.
// It reads the current track from the PlaybackService and publishes through the
// SocialGateway. This is metadata, never audio — independent of the two-engine
// invariant.
type SocialService interface {
	// Platforms lists the platforms the UI can target (canonical order).
	Platforms() []social.Platform
	// AnnounceNowPlaying posts the current track to each platform, returning one
	// receipt per platform. Returns ErrNothingPlaying when nothing is on the air,
	// social.ErrBadPlatform for an unknown platform, or a gateway error (fail-fast).
	AnnounceNowPlaying(ctx context.Context, platforms []social.Platform) ([]social.Receipt, error)
	// PostCustom publishes a free-text note to one platform. Returns
	// social.ErrEmptyText for blank text or social.ErrBadPlatform for an unknown
	// platform.
	PostCustom(ctx context.Context, p social.Platform, text string) (social.Receipt, error)
}

// StaffScheduleService manages the announcer roster (the human grid, Story 9.1):
// which announcer is on air in which weekday time slot. It is organizational
// metadata — no audio, independent of the two-engine invariant.
type StaffScheduleService interface {
	// AddShift registers a new shift. The id is generated internally. Returns a
	// validation sentinel (staffschedule.ErrEmptyAnnouncer/ErrBadWeekday/
	// ErrBadRange) for a bad shift, or staffschedule.ErrOverlap when the new
	// shift intersects an existing one on the same weekday.
	AddShift(ctx context.Context, announcerID, announcerName string, weekday time.Weekday, startMin, endMin int) (staffschedule.Shift, error)
	// RemoveShift deletes a shift by id (outbound.ErrShiftNotFound if unknown).
	RemoveShift(ctx context.Context, id string) error
	// Roster returns the full validated roster ordered by (weekday, start).
	Roster(ctx context.Context) (staffschedule.Roster, error)
	// OnAir returns the shift covering the given instant (using the shift's
	// weekday/minute-of-day), and false when nobody is scheduled then.
	OnAir(ctx context.Context, at time.Time) (staffschedule.Shift, bool, error)
}

// ScheduleService manages the station's programming templates. Story 4.1 covers
// clocks (hour templates); the weekly grid, playlist generation and events land
// in later Épico 4 stories.
type ScheduleService interface {
	// Clocks lists the station's hour templates.
	Clocks(ctx context.Context) ([]schedule.Clock, error)
	// SaveClock validates and upserts a clock (id-keyed), returning the stored
	// value. Returns a schedule validation error (unchanged store) when invalid.
	SaveClock(ctx context.Context, c schedule.Clock) (schedule.Clock, error)
	// DeleteClock removes a clock by id. Returns outbound.ErrClockNotFound when
	// the id is unknown.
	DeleteClock(ctx context.Context, id string) error
	// Grid returns the station's weekly grid (empty when never saved).
	Grid(ctx context.Context) (schedule.Grid, error)
	// SaveGrid validates the grid against the known clocks and persists it,
	// returning the stored value. Returns a schedule validation error (unchanged
	// store) when a cell is malformed or references an unknown clock.
	SaveGrid(ctx context.Context, g schedule.Grid) (schedule.Grid, error)
	// GeneratePlaylist materializes the clock into an ordered playlist of tracks
	// obeying the rotation rules (no tracks are played — preview only). Returns
	// outbound.ErrClockNotFound for an unknown clock and schedule.ErrEmptyCategory
	// when the catalog produces nothing.
	GeneratePlaylist(ctx context.Context, clockID string) ([]library.Track, error)
	// Events lists the station's scheduled events (fixed-slot programming).
	Events(ctx context.Context) ([]schedule.ScheduledEvent, error)
	// SaveEvent validates and upserts a scheduled event (id-keyed), returning the
	// stored value. Returns a schedule validation error (unchanged store) when
	// invalid.
	SaveEvent(ctx context.Context, e schedule.ScheduledEvent) (schedule.ScheduledEvent, error)
	// DeleteEvent removes an event by id. Returns outbound.ErrEventNotFound when
	// the id is unknown.
	DeleteEvent(ctx context.Context, id string) error
	// NextInjections returns the media a station should inject at instant at:
	// the events due then (schedule.DueEvents, priority-desc), resolved to local
	// mixable/PCM tracks via each event's MediaRef. Events with an empty MediaRef
	// (generated later, e.g. TTS) are skipped. Never resolves to Spotify.
	NextInjections(ctx context.Context, at time.Time) ([]library.Track, error)
}

// AutopilotStatus is the autopilot's observable state for the UI toggle:
// whether unattended supervision is running, the current queue depth, and when
// the last tick ran (zero time when it never ticked). Tags JSON camelCase.
type AutopilotStatus struct {
	Enabled  bool      `json:"enabled"`
	QueueLen int       `json:"queueLen"`
	LastTick time.Time `json:"lastTick"`
}

// AutopilotService supervises unattended playout (Story 4.5). Each Tick injects
// the events due now in front of the queue (priority order), refills the queue
// from the current grid hour when it runs low, and — the épico invariant, never
// mute — falls back to an emergency catalog shuffle rather than let the station
// go silent. SetEnabled(true) runs one Tick immediately so the queue starts
// filling without waiting for the loop interval.
/*
StemService prepara o INSTRUMENTAL de uma faixa local com separação por IA.

É a única resposta para faixa MONO: o modo VOZ (cancelamento de centro) precisa
de imagem estéreo, o modelo separa por conteúdo.

É PRÉ-RENDER, nunca caminho do playout: leva ~1,4× o tempo real (medido). O
operador pede, o worker prepara, o arquivo fica guardado.
*/
type StemService interface {
	// Available diz se a ferramenta existe nesta máquina. Sem ela, a UI esconde a
	// função em vez de falhar no clique.
	Available() bool
	// Prepare enfileira a separação. Idempotente: pronto devolve pronto, em
	// andamento não duplica.
	Prepare(ctx context.Context, id string) (string, error)
	// Status devolve o estado atual e, quando falhou, o motivo.
	Status(ctx context.Context, id string) (string, string)
	// PrepareAll enfileira toda faixa local ainda sem instrumental e devolve
	// quantas entraram. Compromete a máquina por horas — só é chamado quando o
	// operador LIGA o preparo automático, nunca por conta própria.
	PrepareAll(ctx context.Context) int
	// Working devolve o que está na fila ou rodando AGORA, por id. É o que a
	// LISTA consulta: perguntar faixa a faixa daria uma requisição por linha.
	// Mapa vazio = nada em andamento, e é o sinal de parada do poll.
	Working(ctx context.Context) map[string]string
}

type AutopilotService interface {
	// Status reports the current enabled flag, queue depth and last tick time.
	Status(ctx context.Context) AutopilotStatus
	// SetEnabled toggles supervision. Enabling ticks once immediately; the
	// returned status reflects the post-tick state. Propagates a tick error.
	SetEnabled(ctx context.Context, enabled bool) (AutopilotStatus, error)
	// Tick runs one supervision cycle (idempotent, safe to loop). Content
	// failures degrade to the emergency playlist, never to silence.
	Tick(ctx context.Context) error
}

// ContentService serves the station's canned UI catalogs (operator screens,
// web-panel tabs, program preview) so the frontend carries no hardcoded lists.
type ContentService interface {
	// Operational returns the Operacional-screen catalog.
	Operational(ctx context.Context) content.Operational
	// WebTabs returns the Internet side-panel tab catalog.
	WebTabs(ctx context.Context) []content.WebTab
	// Program returns the mock program preview rows.
	Program(ctx context.Context) content.Program
	// GradeLoad returns the per-hour grade occupancy for the 24h histogram.
	GradeLoad(ctx context.Context) content.GradeLoad
}
