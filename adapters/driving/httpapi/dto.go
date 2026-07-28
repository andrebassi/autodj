package httpapi

import (
	"autodj/domain/library"
	"autodj/domain/playback"
	"autodj/ports/inbound"
)

// trackDTO is the JSON shape returned for a catalog track. It surfaces the
// capability flags so the frontend can gate PCM-only features (crossfade,
// beatmatch, waveform) per the two-engine matrix.
type trackDTO struct {
	ID         string  `json:"id"`
	Title      string  `json:"title"`
	Artist     string  `json:"artist"`
	Album      string  `json:"album"`
	Category   string  `json:"category"`
	Source     string  `json:"source"`
	DurationMs int64   `json:"duration_ms"`
	BPM        float64 `json:"bpm"`
	Key        string  `json:"key"`
	// LoudnessLUFS é o loudness integrado medido (0 = não medido). Alimenta o
	// ganho por faixa no playout e o selo de atenuação no deck.
	LoudnessLUFS float64     `json:"loudness_lufs"`
	Mixable      bool        `json:"mixable"`
	PCM          bool        `json:"pcm_available"`
	Analyzed     bool        `json:"analyzed"` // BPM/key analysis has run (PCM only)
	Markers      []markerDTO `json:"markers"`
	// SpotifyURI is the spotify:track:… deep link (spotify source only) — lets the
	// UI open the song on the platform. PlayCount is the lifetime play tally. Both
	// are presentational (detail popup), never affect mix/broadcast rules.
	SpotifyURI string `json:"spotify_uri,omitempty"`
	// YouTubeURI is the youtube video id / URL (youtube source only) — lets the UI
	// open the video on the platform. Presentational, never affects mix/broadcast.
	YouTubeURI string `json:"youtube_uri,omitempty"`
	PlayCount  int64  `json:"play_count"`
	// InstrumentalReady: já existe arquivo SEM VOCAL preparado por IA para esta
	// faixa. Vem daqui (lido fresco do repo) e NÃO do Playable: aquele é um
	// retrato do instante do enfileiramento, e a separação termina minutos depois
	// — o retrato diria "não" para sempre.
	InstrumentalReady bool `json:"instrumental_ready"`
	// Folders são as pastas efetivas (do operador, ou a do serviço em streaming).
	Folders []string `json:"folders"`
}

// markerDTO is the wire shape of a cue marker (kind + position + label).
// `auto` diz se a marca veio do ANALISADOR ou da mão do operador — a UI precisa
// disso para desenhá-las diferente e para avisar que uma re-análise substitui
// só as automáticas.
type markerDTO struct {
	Kind  string `json:"kind"`
	PosMs int64  `json:"pos_ms"`
	Label string `json:"label"`
	Auto  bool   `json:"auto"`
}

func toMarkerDTOs(ms []library.Marker) []markerDTO {
	out := make([]markerDTO, 0, len(ms))
	for _, m := range ms {
		out = append(out, markerDTO{
			Kind: string(m.Kind), PosMs: m.PosMs, Label: m.Label, Auto: m.Auto,
		})
	}
	return out
}

func toTrackDTO(t *library.Track) trackDTO {
	return trackDTO{
		ID: t.ID, Title: t.Title, Artist: t.Artist, Album: t.Album,
		Category: t.Category, Source: string(t.Source),
		DurationMs: t.Duration.Milliseconds(), BPM: t.BPM, Key: t.MusicalKey,
		LoudnessLUFS: t.LoudnessLUFS,
		Mixable:      t.Mixable, PCM: t.PCMAvailable, Analyzed: t.Analyzed(),
		Markers:    toMarkerDTOs(t.Markers),
		SpotifyURI: t.SpotifyURI, YouTubeURI: t.YouTubeURI, PlayCount: t.PlayCount,
		// Folders EFETIVAS: streaming devolve a pasta do serviço (derivada da
		// fonte), local devolve as do operador. A UI filtra por este campo e não
		// precisa saber a regra — se ela tivesse que decidir, a lista e o filtro
		// divergiriam na primeira mudança.
		InstrumentalReady: t.HasInstrumental(),
		Folders:           emptyIfNil(library.TrackFolders(*t)),
	}
}

// emptyIfNil garante `[]` no JSON em vez de `null`: o front faz .filter/.includes
// direto no campo, e null estoura.
func emptyIfNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func toTrackDTOs(ts []*library.Track) []trackDTO {
	out := make([]trackDTO, 0, len(ts))
	for _, t := range ts {
		out = append(out, toTrackDTO(t))
	}
	return out
}

// waveformDTO is the wire shape of the waveform-editor payload: duration (to
// map peaks/markers to time) + the normalized [0,1] peaks.
type waveformDTO struct {
	DurationMs int64     `json:"duration_ms"`
	Peaks      []float64 `json:"peaks"`
}

func toWaveformDTO(wf inbound.WaveformData) waveformDTO {
	peaks := wf.Peaks
	if peaks == nil {
		peaks = []float64{}
	}
	return waveformDTO{DurationMs: wf.DurationMs, Peaks: peaks}
}

// categoryDTO is the wire shape of a media category for the picker: the domain
// key plus its pt-BR label and icon. Labels/icons are presentation and live
// here (delivery layer), not in the domain — a test guards drift vs the domain
// category set.
type categoryDTO struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Ico   string `json:"ico"`
}

// categoryLabels maps each domain category to its pt-BR label + icon. Must
// stay in sync with library.Categories() (guarded by TestCategoryLabelParity).
var categoryLabels = map[library.Category]categoryDTO{
	library.CategoryMusic:       {"music", "Músicas", "🎵"},
	library.CategoryJingle:      {"jingle", "Vinhetas", "⭐"},
	library.CategorySoundtrack:  {"soundtrack", "Trilhas", "🎬"},
	library.CategoryEffect:      {"effect", "Efeitos", "✳️"},
	library.CategoryCommercial:  {"commercial", "Comerciais", "📢"},
	library.CategoryPromo:       {"promo", "Programetes", "🎙"},
	library.CategorySponsorship: {"sponsorship", "Oferecimentos", "❤️"},
	library.CategoryExtra:       {"extra", "Extras", "🗂"},
}

func toCategoryDTOs(cs []library.Category) []categoryDTO {
	out := make([]categoryDTO, 0, len(cs))
	for _, c := range cs {
		if d, ok := categoryLabels[c]; ok {
			out = append(out, d)
		} else {
			out = append(out, categoryDTO{Key: string(c), Label: string(c), Ico: "🗂"})
		}
	}
	return out
}

// playableDTO é o espelho REST do mesmo Playable que o WS serve.
//
// ⚠️ SÃO DOIS DTOs (este e ws/dto.go) e eles têm de andar juntos: a UI ao vivo
// lê o do WS, então um campo que entre só aqui simplesmente não chega ao deck —
// sem erro, sem aviso.
type playableDTO struct {
	TrackID    string `json:"track_id"`
	Title      string `json:"title"`
	Artist     string `json:"artist"`
	Source     string `json:"source"`
	URI        string `json:"uri"`
	DurationMs int64  `json:"duration_ms"`
	Origin     string `json:"origin"`
	ArtworkURL string `json:"artwork_url"`
	Category   string `json:"category"`
	// Dicas de reprodução (só faixa local/PCM; streaming vem tudo zero).
	// BPM é ESTÁTICO (uma vez medido não muda), então pode viajar no retrato do
	// enfileiramento. O estado do SEM-VOZ não pode — termina minutos depois.
	BPM          float64 `json:"bpm"`
	CueInMs      int64   `json:"cue_in_ms"`
	CueOutMs     int64   `json:"cue_out_ms"`
	VocalMs      int64   `json:"vocal_ms"`
	LoudnessLUFS float64 `json:"loudness_lufs"`
	// Dá pra tirar o vocal por cancelamento de centro? (imagem estéreo medida
	// acima do limiar do domínio). Streaming e faixa não analisada vêm false.
}

func toPlayableDTO(p playback.Playable) playableDTO {
	return playableDTO{
		TrackID: p.TrackID, Title: p.Title, Artist: p.Artist, Source: string(p.Source),
		URI: p.URI, DurationMs: p.Duration.Milliseconds(), Origin: p.Origin,
		ArtworkURL: p.ArtworkURL, Category: p.Category,
		CueInMs: p.CueInMs, CueOutMs: p.CueOutMs, VocalMs: p.VocalMs,
		BPM: p.BPM, LoudnessLUFS: p.LoudnessLUFS,
	}
}

type queueItemDTO struct {
	Playable   playableDTO `json:"playable"`
	Transition string      `json:"transition"`
}

// nowPlayingMsg is the payload broadcast over WebSocket and returned by the
// nowplaying endpoint — the single shape the Player UI consumes.
type nowPlayingMsg struct {
	Type       string         `json:"type"`
	NowPlaying playableDTO    `json:"now_playing"`
	Queue      []queueItemDTO `json:"queue"`
}

func nowPlayingDTO(np playback.Playable, q []playback.QueueItem) nowPlayingMsg {
	items := make([]queueItemDTO, 0, len(q))
	for _, it := range q {
		items = append(items, queueItemDTO{
			Playable:   toPlayableDTO(it.Playable),
			Transition: string(it.Transition),
		})
	}
	return nowPlayingMsg{Type: "nowplaying", NowPlaying: toPlayableDTO(np), Queue: items}
}
