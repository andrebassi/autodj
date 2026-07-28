// Package outbound declares the driven-side interfaces the domain depends on.
// Implementations live in adapters/driven. The domain never imports these
// adapters — direction is adapters → ports → domain.
package outbound

import (
	"context"
	"errors"
	"io"
	"time"

	"autodj/domain/aidj"
	"autodj/domain/cartwall"
	"autodj/domain/identity"
	"autodj/domain/library"
	"autodj/domain/linein"
	"autodj/domain/mixing"
	"autodj/domain/operatorprofile"
	"autodj/domain/playback"
	"autodj/domain/playlog"
	"autodj/domain/rds"
	"autodj/domain/requests"
	"autodj/domain/schedule"
	"autodj/domain/social"
	"autodj/domain/spotify"
	"autodj/domain/staffschedule"
	"autodj/domain/youtube"
)

// TrackRepo persists and queries the catalog.
type TrackRepo interface {
	Upsert(ctx context.Context, t *library.Track) error
	List(ctx context.Context) ([]*library.Track, error)
	Get(ctx context.Context, id string) (*library.Track, error)
	Count(ctx context.Context) (int, error)
	// Delete removes a track by id. Returns an error if it does not exist.
	Delete(ctx context.Context, id string) error
	// DeleteAll wipes the whole catalog (header "Limpar"). Idempotent.
	DeleteAll(ctx context.Context) error
}

// FolderCatalog guarda os NOMES das pastas criadas pelo operador. Existe só por
// causa da pasta recém-criada e ainda vazia: as pastas em uso já saem das faixas,
// mas uma pasta sem faixa nenhuma precisa de onde morar até receber a primeira,
// senão "criar pasta" sumiria no reload. Implementações: sqlite, memory.
type FolderCatalog interface {
	List(ctx context.Context) ([]string, error)
	// Create é idempotente e ignora caixa ("Rock" == "rock").
	Create(ctx context.Context, name string) error
	// Delete remove só do catálogo; nunca apaga faixa. Idempotente.
	Delete(ctx context.Context, name string) error
}

// MediaStore persists uploaded media files. It abstracts the local filesystem
// (filestore adapter, ~/.autodj/media) so upload/delete stays out of the app
// service and can be swapped for object storage later.
type MediaStore interface {
	// Save writes r under a sanitized name derived from filename and returns
	// the stored file path.
	Save(ctx context.Context, filename string, r io.Reader) (string, error)
	// Remove deletes the file at path. Missing files are not an error.
	Remove(ctx context.Context, path string) error
}

// TTSGateway synthesizes speech from text, out-of-process (ElevenLabs HTTP or
// Piper exec). It returns the audio bytes; the app persists them via MediaStore
// (cache-by-hash) so the same line+voice is never synthesized twice. The result
// is PCM — a source=tts track that is mixable on the local bus, which is exactly
// what lets the AI DJ voice bridge a DRM Spotify track without DRM audio crossing.
type TTSGateway interface {
	// Synthesize renders text in voice to audio bytes (wav/pcm). The reader is
	// the whole synthesized clip; the caller stores and closes it. Returns a
	// non-nil error on gateway failure.
	Synthesize(ctx context.Context, text string, voice aidj.Voice) (io.ReadCloser, error)
}

// MetadataExtractor reads tags/duration/bitrate from a media file (ffprobe).
type MetadataExtractor interface {
	Extract(ctx context.Context, path string) (Metadata, error)
}

// Metadata is the subset of media info the scanner needs to build a Track.
type Metadata struct {
	Title    string
	Artist   string
	Album    string
	Duration time.Duration
	Bitrate  int
}

// FolderScanner walks a directory and yields audio file paths.
type FolderScanner interface {
	Scan(ctx context.Context, root string) ([]string, error)
}

// StemSeparator separates the vocal from a PCM file with an AI model (Demucs),
// out-of-process, and writes the vocal-free result to outPath.
//
// É a ÚNICA coisa que resolve faixa MONO: cancelamento de centro precisa de
// imagem estéreo, o modelo não — ele separa por conteúdo.
//
// É LENTO por natureza: ~1,4× o tempo real medido num M1 Pro com 6 threads (45s
// de áudio em 64,8s), ou seja ~6 min para uma música de 4. Por isso nunca é
// chamado no caminho do playout — só por um worker, fora da hot path, com o
// resultado guardado em arquivo.
//
// `Available` diz se a ferramenta existe nesta máquina; sem ela o recurso some da
// UI em vez de falhar no clique (mesma degradação graciosa do AudioAnalyzer).
type StemSeparator interface {
	Available() bool
	// SeparateVocals lê `path` e escreve em `outPath` o áudio sem vocal.
	SeparateVocals(ctx context.Context, path, outPath string) error
}

// AudioAnalyzer computes BPM/key/waveform from a PCM file, out-of-process
// (aubio/keyfinder/audiowaveform CLIs). It only ever receives a local file
// path — DRM sources are never analyzed — and degrades gracefully: a missing
// tool yields a zero field rather than an error.
type AudioAnalyzer interface {
	Analyze(ctx context.Context, path string) (library.Analysis, error)
}

// WaveformGenerator produces the peaks used by the waveform editor: bars
// normalized amplitudes in [0,1] for a PCM track. It only ever receives a PCM
// track — the app layer gates DRM sources upstream (ErrWaveformNotPCM) — so the
// two-engine invariant holds. Today this is a deterministic placeholder derived
// from the track id; later it reads the peaks JSON the analyzer precomputes.
type WaveformGenerator interface {
	Peaks(ctx context.Context, t *library.Track, bars int) ([]float64, error)
}

// MixSettingsRepo persists the station's single fine-mixing configuration.
// Get returns the stored settings, or mixing.Default() when none has ever been
// saved (never an "empty" error). Save replaces the whole value (single-row).
type MixSettingsRepo interface {
	Get(ctx context.Context) (mixing.Settings, error)
	Save(ctx context.Context, s mixing.Settings) error
}

// ClockRepo persists and queries the station's clocks (hour templates). Unlike
// MixSettingsRepo this is multi-row: id is the key. Get returns ErrClockNotFound
// when the id is absent; Save upserts by id.
type ClockRepo interface {
	List(ctx context.Context) ([]schedule.Clock, error)
	Get(ctx context.Context, id string) (schedule.Clock, error)
	Save(ctx context.Context, c schedule.Clock) error
	Delete(ctx context.Context, id string) error
}

// ErrClockNotFound is returned by ClockRepo.Get/Delete for an unknown clock id.
var ErrClockNotFound = errors.New("outbound: clock not found")

// EventRepo persists and queries the station's scheduled events (fixed-slot
// programming that preempts the music rotation). Multi-row: id is the key. Get
// returns ErrEventNotFound when the id is absent; Save upserts by id.
type EventRepo interface {
	List(ctx context.Context) ([]schedule.ScheduledEvent, error)
	Get(ctx context.Context, id string) (schedule.ScheduledEvent, error)
	Save(ctx context.Context, e schedule.ScheduledEvent) error
	Delete(ctx context.Context, id string) error
}

// ErrEventNotFound is returned by EventRepo.Get/Delete for an unknown event id.
var ErrEventNotFound = errors.New("outbound: event not found")

// RequestRepo persists and queries listener requests / dedications (the audience
// moderation queue). Multi-row: id is the key. Get returns ErrRequestNotFound
// when the id is absent; Save upserts by id; List returns all ordered by id.
type RequestRepo interface {
	Save(ctx context.Context, r requests.Request) error
	Get(ctx context.Context, id string) (requests.Request, error)
	List(ctx context.Context) ([]requests.Request, error)
}

// ErrRequestNotFound is returned by RequestRepo.Get for an unknown request id.
var ErrRequestNotFound = errors.New("outbound: request not found")

// StaffScheduleRepo persists and queries the announcer roster (the human grid).
// Multi-row: id is the key. Get returns ErrShiftNotFound when absent; Save
// upserts by id; Delete removes by id (ErrShiftNotFound if unknown); List
// returns all shifts ordered by (weekday, start).
type StaffScheduleRepo interface {
	Save(ctx context.Context, s staffschedule.Shift) error
	Get(ctx context.Context, id string) (staffschedule.Shift, error)
	Delete(ctx context.Context, id string) error
	List(ctx context.Context) ([]staffschedule.Shift, error)
}

// ErrShiftNotFound is returned by StaffScheduleRepo.Get/Delete for an unknown id.
var ErrShiftNotFound = errors.New("outbound: shift not found")

// GridRepo persists the station's weekly grid. It is single-row (one grid per
// station): Get returns an empty Grid when none was ever saved; Save replaces it.
type GridRepo interface {
	Get(ctx context.Context) (schedule.Grid, error)
	Save(ctx context.Context, g schedule.Grid) error
}

// CartwallRepo persists the operator's cartwall board. It is single-row (one
// board per user): Load returns an empty Board when none was ever saved; Save
// replaces the whole board.
type CartwallRepo interface {
	Load(ctx context.Context) (cartwall.Board, error)
	Save(ctx context.Context, b cartwall.Board) error
}

// SpotifyAccountRepo persists the operator's linked Spotify OAuth session. It is
// single-row (one account per user): Load returns spotify.ErrNoAccount when none
// was ever saved; Save replaces the whole account; Delete unlinks it.
type SpotifyAccountRepo interface {
	Load(ctx context.Context) (spotify.Account, error)
	Save(ctx context.Context, a spotify.Account) error
	Delete(ctx context.Context) error
}

// SpotifyOAuth talks to accounts.spotify.com over the network: it builds the
// authorize URL and exchanges/refreshes tokens. PKCE (verifier/challenge) is the
// adapter's concern — the domain never sees crypto. The app service passes the
// state and PKCE code_challenge it wants embedded in the URL.
type SpotifyOAuth interface {
	// AuthURL returns the accounts.spotify.com/authorize URL carrying the client
	// id, redirect, state, PKCE challenge (S256) and scopes.
	AuthURL(state, challenge string, scopes []string) string
	// NewPKCE generates a fresh PKCE (verifier, code_challenge) pair. Verifier is
	// a high-entropy random string; challenge = base64url(sha256(verifier)).
	NewPKCE() (verifier, challenge string, err error)
	// NewState generates a random CSRF state nonce for the authorize round-trip.
	NewState() (string, error)
	// Exchange trades an authorization code (+ the PKCE verifier) for an Account.
	Exchange(ctx context.Context, code, verifier string) (spotify.Account, error)
	// Refresh trades a refresh token for a fresh Account. Spotify may omit a new
	// refresh token — the adapter preserves the old one.
	Refresh(ctx context.Context, refreshToken string) (spotify.Account, error)
	// Probe consulta perfil e devices com o token dado e devolve os status HTTP
	// crus. É o diagnóstico: distingue conta fora da allowlist (403), token morto
	// (401) e sessão saudável (200). Nunca inicia reprodução.
	Probe(ctx context.Context, accessToken string) (spotify.Probe, error)
}

// SpotifyCatalog reads the Spotify Web API (api.spotify.com/v1) for metadata
// only — search, playlist contents and track lookup. It never returns audio
// (DRM): importing its items yields non-mixable Spotify tracks. The access token
// is supplied per call (the app service refreshes it transparently). Post-Nov
// 2024 the API no longer exposes audio-features/analysis, so no BPM/key here.
type SpotifyCatalog interface {
	// Search finds tracks or playlists matching query. kind is spotify.KindTrack
	// or spotify.KindPlaylist.
	Search(ctx context.Context, token, query, kind string) ([]spotify.CatalogItem, error)
	// PlaylistTracks lists the tracks of a playlist (metadata only).
	PlaylistTracks(ctx context.Context, token, playlistID string) ([]spotify.CatalogItem, error)
	// Tracks resolves track URIs/ids to their metadata (for import by URI).
	Tracks(ctx context.Context, token string, uris []string) ([]spotify.CatalogItem, error)
}

// YouTubeAccountRepo persists the operator's linked Google/YouTube OAuth session.
// It is single-row (one account per user): Load returns youtube.ErrNoAccount when
// none was ever saved; Save replaces the whole account; Delete unlinks it.
type YouTubeAccountRepo interface {
	Load(ctx context.Context) (youtube.Account, error)
	Save(ctx context.Context, a youtube.Account) error
	Delete(ctx context.Context) error
}

// YouTubeOAuth talks to accounts.google.com over the network: it builds the
// authorize URL and exchanges/refreshes tokens using the auth-code flow WITH a
// client secret (not PKCE). The app service passes the state it wants embedded in
// the URL.
type YouTubeOAuth interface {
	// AuthURL returns the accounts.google.com authorize URL carrying the client
	// id, redirect, state, scopes, access_type=offline and prompt=consent (so a
	// refresh token is issued).
	AuthURL(state string, scopes []string) string
	// NewState generates a random CSRF state nonce for the authorize round-trip.
	NewState() (string, error)
	// Exchange trades an authorization code (+ the client secret, held by the
	// adapter) for an Account.
	Exchange(ctx context.Context, code string) (youtube.Account, error)
	// Refresh trades a refresh token for a fresh Account. Google may omit a new
	// refresh token — the adapter preserves the old one.
	Refresh(ctx context.Context, refreshToken string) (youtube.Account, error)
}

// YouTubeCatalog reads the YouTube Data API v3 for metadata only — search,
// playlist contents and video lookup. It never returns audio (embed/DRM):
// importing its items yields non-mixable YouTube tracks. It authenticates with an
// API KEY (not the OAuth token), so the token argument mirrors SpotifyCatalog for
// shape parity but is unused.
type YouTubeCatalog interface {
	// Search finds videos or playlists matching query. kind is youtube.KindVideo
	// or youtube.KindPlaylist.
	Search(ctx context.Context, token, query, kind string) ([]youtube.CatalogItem, error)
	// PlaylistItems lists the videos of a playlist (metadata only).
	PlaylistItems(ctx context.Context, token, playlistID string) ([]youtube.CatalogItem, error)
	// Videos resolves video ids/URLs to their metadata (for import by URI).
	Videos(ctx context.Context, token string, ids []string) ([]youtube.CatalogItem, error)
}

// PlaybackEngine drives the actual playout player (Liquidsoap over a Unix
// socket). It only ever handles mixable (PCM) sources — the queue guarantees
// this upstream.
type PlaybackEngine interface {
	// Push appends an item to the engine's playout queue.
	Push(ctx context.Context, item playback.QueueItem) error
	// Skip advances to the next track immediately.
	Skip(ctx context.Context) error
	// RemoveAt drops the pending item at index i (0 = the next queued item after
	// the current track). Out-of-range i is a no-op. Keeps the engine's pending
	// queue aligned with the service mirror when the operator removes an item.
	RemoveAt(ctx context.Context, i int) error
	// MoveAt reorders the pending list: the item at index from goes to index to
	// (both 0 = the next queued item after the current track). Out-of-range is a
	// no-op; from==to is a no-op. Keeps the engine's pending queue aligned with
	// the service mirror when the operator nudges an item up/down.
	MoveAt(ctx context.Context, from, to int) error
	// NowPlaying reports what the engine is currently playing.
	NowPlaying(ctx context.Context) (playback.Playable, error)
	// Healthy reports whether the engine process/socket is responsive.
	Healthy(ctx context.Context) bool
}

// RDSEncoder drives the FM sidecar that broadcasts the Radio Data System stream
// (PI/PS/PTY/RadioText). It transports *metadata only* — no audio — so pushing a
// state naming a DRM Spotify track never breaches the two-engine invariant.
// Implementations: a file encoder (writes the state for a hardware/software RDS
// tool to pick up) or a no-op stub.
type RDSEncoder interface {
	// Push writes the full RDS state to the encoder. The state is pre-validated by
	// the app service.
	Push(ctx context.Context, state rds.State) error
}

// LineInController commands the live-capture inputs on the playout engine
// (Story 7.2): arming/disarming a hardware source onto the local bus and setting
// its gain/duck. Only ever handles PCM (line-in) sources. Implementations: a
// Liquidsoap socket controller (input.harbor/alsa + amplify) or a no-op stub.
type LineInController interface {
	// Arm routes the input onto the bus at its gain, ducking the music by its duck.
	Arm(ctx context.Context, in linein.Input) error
	// Disarm removes the input id from the bus.
	Disarm(ctx context.Context, id string) error
	// SetLevels updates the gain/duck of an already-armed input.
	SetLevels(ctx context.Context, in linein.Input) error
}

// Clock abstracts time for deterministic testing.
type Clock interface {
	Now() time.Time
}

// ErrUserNotFound is returned by UserRepo.Get/GetByName/Delete for an unknown user.
var ErrUserNotFound = errors.New("outbound: user not found")

// PasswordHasher hashes and verifies passwords out of the domain (Story 7.3).
// Implementation: argon2id. Plaintext passwords never reach the domain or a log.
type PasswordHasher interface {
	// Hash derives an opaque, self-describing hash (salt+params embedded) from a
	// plaintext password.
	Hash(password string) (string, error)
	// Verify reports whether password matches hash in constant time. A malformed
	// hash simply returns false.
	Verify(password, hash string) bool
}

// UserRepo persists station accounts (Story 7.3). Home use keeps this small and
// global to the station (SQLite), not per-user.
type UserRepo interface {
	// Get returns the user by id. Returns ErrUserNotFound when unknown.
	Get(ctx context.Context, id string) (identity.User, error)
	// GetByName returns the user by (unique) name. Returns ErrUserNotFound when unknown.
	GetByName(ctx context.Context, name string) (identity.User, error)
	// List returns every account.
	List(ctx context.Context) ([]identity.User, error)
	// Count reports how many accounts exist (used to decide the boot seed).
	Count(ctx context.Context) (int, error)
	// Upsert inserts or updates a user keyed by id.
	Upsert(ctx context.Context, u identity.User) error
	// Delete removes a user by id. Returns ErrUserNotFound when unknown.
	Delete(ctx context.Context, id string) error
}

// ErrProfileNotFound is returned by ProfileRepo.Get for a user with no saved
// layout yet. The app service maps it to operatorprofile.Default.
var ErrProfileNotFound = errors.New("outbound: profile not found")

// ProfileRepo persists each operator's player layout (Story 7.4). Multi-row:
// user id is the key. Get returns ErrProfileNotFound when the user has never
// saved a layout; Upsert replaces the whole profile.
type ProfileRepo interface {
	// Get returns the profile for userID. Returns ErrProfileNotFound when absent.
	Get(ctx context.Context, userID string) (operatorprofile.Profile, error)
	// Upsert inserts or replaces the profile keyed by its UserID.
	Upsert(ctx context.Context, p operatorprofile.Profile) error
}

// PlayLogRepo persists the station's play history (Story 7.5). It is
// APPEND-ONLY by design — there is no Update or Delete. The history is the
// source of truth for ECAD reports and the irradiation proof, so once an entry
// is written it is immutable. Append is idempotent on the entry ID (re-writing
// the same play is a no-op) so a retried record never double-counts.
type PlayLogRepo interface {
	// Append writes one play entry. Idempotent on Entry.ID.
	Append(ctx context.Context, e playlog.Entry) error
	// Since returns every entry with PlayedAt in [from, to), oldest first.
	Since(ctx context.Context, from, to time.Time) ([]playlog.Entry, error)
	// Top returns the n most played tracks whose PlayedAt is in [from, to),
	// ordered by count descending.
	Top(ctx context.Context, from, to time.Time, n int) ([]playlog.TopItem, error)
}

// SocialGateway publishes a composed Post to a social network, out-of-process
// (Story 8.5). The home build wires a stub that returns a synthetic receipt with
// no external call; a real gateway (WhatsApp Cloud API / X / Facebook Graph) is a
// drop-in adapter. This carries metadata, never audio.
type SocialGateway interface {
	// Publish sends post to its platform and returns the receipt (external id +
	// url + timestamp). A non-nil error means the post did not go out.
	Publish(ctx context.Context, post social.Post) (social.Receipt, error)
}

// Claims is the verified payload of an auth token: the user id and role. Kept
// minimal — the app re-reads the user when it needs more.
type Claims struct {
	UserID string
	Role   identity.Role
}

// TokenIssuer mints and parses short-lived session tokens (Story 7.3).
// Implementation: signed JWT with an expiry.
type TokenIssuer interface {
	// Issue signs a token carrying the user's id and role, with the issuer's fixed
	// TTL.
	Issue(u identity.User) (string, error)
	// Parse validates the signature and expiry and returns the claims. Returns a
	// non-nil error for a missing/malformed/tampered/expired token.
	Parse(token string) (Claims, error)
}
