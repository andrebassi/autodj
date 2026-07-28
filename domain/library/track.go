// Package library models the music/media catalog. A Track is the durable
// catalog record; Playable (domain/playback) is the transient thing the engine
// plays. Track carries the load-bearing SourceKind and derived capability flags.
package library

import (
	"errors"
	"time"

	"autodj/domain/shared"
)

var (
	// ErrEmptyID is returned when a Track has no identifier.
	ErrEmptyID = errors.New("track: id required")
	// ErrBadSource is returned for an unknown SourceKind.
	ErrBadSource = errors.New("track: invalid source kind")
	// ErrLocalNeedsPath is returned when a local track has no file path.
	ErrLocalNeedsPath = errors.New("track: local source requires path")
	// ErrSpotifyNeedsURI is returned when a spotify track has no uri.
	ErrSpotifyNeedsURI = errors.New("track: spotify source requires uri")
	// ErrYouTubeNeedsURI is returned when a youtube track has no uri.
	ErrYouTubeNeedsURI = errors.New("track: youtube source requires uri")
	// ErrBadDuration is returned for a non-positive duration.
	ErrBadDuration = errors.New("track: duration must be positive")
	// ErrBadCategory is returned for a category outside the fixed set.
	ErrBadCategory = errors.New("track: invalid category")
	// ErrTrackNotFound is returned when a track id is not in the catalog. The
	// app service normalizes adapter-specific not-found errors to this so the
	// delivery layer can map it to 404 without importing an adapter.
	ErrTrackNotFound = errors.New("track: not found")
	// ErrNoArtwork is returned when a track has no cover to serve: no external
	// artwork URL (Spotify/YouTube/…) and no embedded picture in its local file.
	// The delivery layer maps it to 404 so the UI falls back to a placeholder.
	ErrNoArtwork = errors.New("track: no artwork")
)

// Track is a catalog entry. Field validity depends on SourceKind via the
// capability matrix — e.g. PCMAvailable/Mixable are derived, never set freely,
// so a Spotify track can never be marked mixable into the local playout bus.
type Track struct {
	ID       string
	Title    string
	Artist   string
	Album    string
	Category string // folder-tree category (rotation bucket), e.g. "music", "commercial"

	// Folders são as pastas do OPERADOR ("rock", "anos 90"), texto livre criado por
	// ele. Uma faixa pode estar em VÁRIAS — na prática é etiqueta, apresentada como
	// pasta. Distinto de Category, que é enum FIXO e alimenta o rodízio da rotação:
	// misturar "rock" ali quebraria a programação.
	//
	// Spotify e YouTube NÃO usam este campo: a pasta deles é derivada de Source, o
	// que nunca desalinha e não exige migrar nada.
	Folders []string

	// ArtworkURL is an external cover image URL for providers that ship one
	// (Spotify album image, YouTube thumbnail, …). Empty for local files, whose
	// cover — when present — is the embedded picture, served on demand by the
	// artwork endpoint. Purely presentational; never affects mix/broadcast rules.
	ArtworkURL string

	Source     shared.SourceKind
	Path       string        // local file path (local/line_in)
	SpotifyURI string        // spotify:track:... (spotify only)
	YouTubeURI string        // youtube video id / URL (youtube only)
	Duration   time.Duration // exact, from ffprobe/analysis
	BPM        float64       // 0 until analyzed; never from Spotify
	MusicalKey string        // Camelot notation; 0/"" until analyzed

	// LoudnessLUFS is the integrated loudness (EBU R128) measured from the PCM
	// file; 0 until analyzed. It is what lets a loud master and a quiet old rip
	// come out at the same perceived level — the playout derives a per-track gain
	// from the distance between this and the operator's target.
	LoudnessLUFS float64


	// InstrumentalPath é o caminho do arquivo SEM VOCAL gerado por separação de
	// IA (Demucs), quando ela já rodou nesta faixa. Vazio = não preparado.
	//
	// É um ARQUIVO NOVO, ao lado do original — a faixa nunca é reescrita, e apagar
	// este arquivo devolve o comportamento anterior. Diferente do modo VOZ, que é
	// cancelamento de centro em tempo real e não gera nada.
	InstrumentalPath string

	WaveformRef string // storage ref to peaks JSON (PCM sources only)
	PlayCount   int64

	Markers []Marker // cue points (PCM sources only); set via SetMarkers

	// Derived, do not set directly — use NewTrack.
	PCMAvailable bool
	Mixable      bool
}

// NewTrack builds a validated Track. It derives PCMAvailable and Mixable from
// the capability matrix so the two-engine invariants (spotify ⇒ !mixable)
// cannot be violated by a caller.
func NewTrack(id string, source shared.SourceKind, title string, dur time.Duration) (*Track, error) {
	if id == "" {
		return nil, ErrEmptyID
	}
	if !source.Valid() {
		return nil, ErrBadSource
	}
	if dur <= 0 {
		return nil, ErrBadDuration
	}
	caps := shared.Capabilities(source)
	return &Track{
		ID:           id,
		Title:        title,
		Source:       source,
		Duration:     dur,
		PCMAvailable: caps.PCM,
		Mixable:      caps.Mixable,
	}, nil
}

// Validate checks source-specific structural invariants after fields are set.
func (t *Track) Validate() error {
	if t.ID == "" {
		return ErrEmptyID
	}
	if !t.Source.Valid() {
		return ErrBadSource
	}
	if t.Duration <= 0 {
		return ErrBadDuration
	}
	switch t.Source {
	case shared.SourceLocal, shared.SourceLineIn:
		if t.Path == "" {
			return ErrLocalNeedsPath
		}
	case shared.SourceSpotify:
		if t.SpotifyURI == "" {
			return ErrSpotifyNeedsURI
		}
	case shared.SourceYouTube:
		if t.YouTubeURI == "" {
			return ErrYouTubeNeedsURI
		}
	}
	// Category is a fixed enum: empty normalizes to the default (scan/seed set
	// it to music), anything outside the set is rejected.
	if t.Category == "" {
		t.Category = string(DefaultCategory)
	}
	if !ValidCategory(Category(t.Category)) {
		return ErrBadCategory
	}
	// Pastas do operador: normaliza aqui pra que NENHUM caminho de escrita (upload,
	// edição, scan, import) consiga gravar duplicata por caixa/espaço ou nome longo.
	folders, err := NormalizeFolders(t.Folders)
	if err != nil {
		return err
	}
	t.Folders = folders
	// Enforce the derived flags regardless of how fields were mutated.
	caps := shared.Capabilities(t.Source)
	t.PCMAvailable = caps.PCM
	t.Mixable = caps.Mixable
	return nil
}

// Analyzed reports whether BPM/key analysis has run. Meaningless (always
// false-ish) for non-PCM sources, which can never be analyzed.
func (t *Track) Analyzed() bool {
	return t.PCMAvailable && t.BPM > 0
}

// HasInstrumental reports whether an AI-separated vocal-free file already exists
// for this track.
//
// Desde a 0.2.32 é a ÚNICA régua do modo VOZ: o cancelamento de centro foi
// aposentado (media 2,0 dB de redução de voz em música real, contra 31,9 dB da
// separação por modelo). O modelo separa por CONTEÚDO, então imagem estéreo não
// entra na conta — nem para permitir, nem para recusar. Importa só o arquivo
// existir, e é por isso que isto funciona em faixa mono.
func (t *Track) HasInstrumental() bool {
	return t.PCMAvailable && t.InstrumentalPath != ""
}

// Separable reports whether AI vocal separation can run on this track: it needs
// PCM on our side. Streaming (Spotify/YouTube) plays out-of-band and there is no
// file to feed the model.
func (t *Track) Separable() bool { return t.PCMAvailable && t.Path != "" }
