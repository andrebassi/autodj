package ws

import "autodj/domain/playback"

// playableDTO é a forma que a UI AO VIVO lê (deck, fila, tocadas).
//
// ⚠️ SÃO DOIS DTOs, este e o de httpapi/dto.go. Campo novo de exibição/dica que
// entre num só some da tela sem erro nenhum — foi assim que o `artist` sumiu do
// deck enquanto a busca do Spotify o mostrava certo. Mexeu aqui, mexa lá.
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
	// Dicas de reprodução (só faixa local/PCM; streaming vem tudo zero):
	// onde o áudio começa/termina de fato, onde a voz entra e quão alto é o
	// master. 0 = desconhecido / nada a fazer.
	// BPM é ESTÁTICO (uma vez medido não muda), então pode viajar no retrato do
	// enfileiramento. O estado do SEM-VOZ não pode — termina minutos depois.
	BPM          float64 `json:"bpm"`
	CueInMs      int64   `json:"cue_in_ms"`
	CueOutMs     int64   `json:"cue_out_ms"`
	VocalMs      int64   `json:"vocal_ms"`
	LoudnessLUFS float64 `json:"loudness_lufs"`
}

type queueItemDTO struct {
	Playable   playableDTO `json:"playable"`
	Transition string      `json:"transition"`
}

type nowPlayingMsg struct {
	Type       string         `json:"type"`
	NowPlaying playableDTO    `json:"now_playing"`
	Queue      []queueItemDTO `json:"queue"`
}

// toPlayableDTO é o ÚNICO ponto de conversão deste pacote. Antes o snapshot
// montava o struct campo a campo em três lugares — e um campo novo esquecido em
// um deles vira bug invisível (a fila mostra, o deck não).
func toPlayableDTO(p playback.Playable) playableDTO {
	return playableDTO{
		TrackID: p.TrackID, Title: p.Title, Artist: p.Artist,
		Source: string(p.Source), URI: p.URI,
		DurationMs: p.Duration.Milliseconds(), Origin: p.Origin,
		ArtworkURL: p.ArtworkURL, Category: p.Category,
		CueInMs: p.CueInMs, CueOutMs: p.CueOutMs, VocalMs: p.VocalMs,
		BPM: p.BPM, LoudnessLUFS: p.LoudnessLUFS,
	}
}

func snapshot(np playback.Playable, q []playback.QueueItem) nowPlayingMsg {
	items := make([]queueItemDTO, 0, len(q))
	for _, it := range q {
		items = append(items, queueItemDTO{
			Playable:   toPlayableDTO(it.Playable),
			Transition: string(it.Transition),
		})
	}
	return nowPlayingMsg{
		Type:       "nowplaying",
		NowPlaying: toPlayableDTO(np),
		Queue:      items,
	}
}
