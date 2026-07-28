package library

import (
	"errors"
	"testing"
	"time"

	"autodj/domain/shared"
)

func localTrack(t *testing.T) *Track {
	t.Helper()
	tr, err := NewTrack("id1", shared.SourceLocal, "Neon Dusk", 3*time.Minute)
	if err != nil {
		t.Fatalf("NewTrack: %v", err)
	}
	tr.Path = "/music/neon-dusk.flac"
	tr.Artist = "Violet Circuit"
	return tr
}

func TestApplyAnalysisPCM(t *testing.T) {
	tr := localTrack(t)
	err := tr.ApplyAnalysis(Analysis{BPM: 128.5, MusicalKey: "8A", WaveformRef: "peaks/id1.json", LoudnessLUFS: -9.2})
	if err != nil {
		t.Fatalf("ApplyAnalysis: %v", err)
	}
	if tr.BPM != 128.5 || tr.MusicalKey != "8A" || tr.WaveformRef != "peaks/id1.json" {
		t.Fatalf("fields not applied: %+v", tr)
	}
	if tr.LoudnessLUFS != -9.2 {
		t.Fatalf("loudness not applied: %v", tr.LoudnessLUFS)
	}
	if !tr.Analyzed() {
		t.Fatal("track should report Analyzed after BPM set")
	}
	// Flags stay consistent (PCM local stays mixable).
	if !tr.PCMAvailable || !tr.Mixable {
		t.Fatalf("flags drifted: pcm=%v mixable=%v", tr.PCMAvailable, tr.Mixable)
	}
}

func TestApplyAnalysisRejectsNonPCM(t *testing.T) {
	tr, err := NewTrack("sp1", shared.SourceSpotify, "DRM Song", 3*time.Minute)
	if err != nil {
		t.Fatalf("NewTrack: %v", err)
	}
	tr.SpotifyURI = "spotify:track:abc"
	if err := tr.ApplyAnalysis(Analysis{BPM: 120}); !errors.Is(err, ErrNotAnalyzable) {
		t.Fatalf("want ErrNotAnalyzable, got %v", err)
	}
	if tr.BPM != 0 {
		t.Fatalf("spotify track must not gain BPM, got %v", tr.BPM)
	}
}

func TestApplyAnalysisRejectsNegativeBPM(t *testing.T) {
	tr := localTrack(t)
	if err := tr.ApplyAnalysis(Analysis{BPM: -1}); !errors.Is(err, ErrBadBPM) {
		t.Fatalf("want ErrBadBPM, got %v", err)
	}
}

func TestApplyAnalysisEmptyDoesNotClobber(t *testing.T) {
	tr := localTrack(t)
	if err := tr.ApplyAnalysis(Analysis{BPM: 128, MusicalKey: "8A", WaveformRef: "peaks/id1.json"}); err != nil {
		t.Fatalf("first apply: %v", err)
	}
	// Second run degraded (only BPM): key/waveform must survive.
	if err := tr.ApplyAnalysis(Analysis{BPM: 130}); err != nil {
		t.Fatalf("second apply: %v", err)
	}
	if tr.MusicalKey != "8A" || tr.WaveformRef != "peaks/id1.json" {
		t.Fatalf("empty fields clobbered prior analysis: %+v", tr)
	}
	if tr.BPM != 130 {
		t.Fatalf("bpm not updated: %v", tr.BPM)
	}
}

// Loudness zero segue a mesma regra dos demais: rodada degradada (ffmpeg sem o
// ebur128, ou medição falhada) NÃO pode apagar a medida boa que já existia — o
// playout passaria a tocar essa faixa sem correção de ganho, em silêncio.
func TestApplyAnalysisZeroLoudnessDoesNotClobber(t *testing.T) {
	tr := localTrack(t)
	if err := tr.ApplyAnalysis(Analysis{BPM: 120, LoudnessLUFS: -14.5}); err != nil {
		t.Fatalf("first apply: %v", err)
	}
	if err := tr.ApplyAnalysis(Analysis{BPM: 121}); err != nil {
		t.Fatalf("second apply: %v", err)
	}
	if tr.LoudnessLUFS != -14.5 {
		t.Fatalf("loudness clobbered: %v", tr.LoudnessLUFS)
	}
}

func TestApplyAnalysisWritesAutoCues(t *testing.T) {
	tr := localTrack(t)
	err := tr.ApplyAnalysis(Analysis{BPM: 120, CueInMs: 1200, VocalMs: 15000, CueOutMs: 175000})
	if err != nil {
		t.Fatalf("ApplyAnalysis: %v", err)
	}
	if got := tr.CueInMs(); got != 1200 {
		t.Fatalf("cue in = %d, want 1200", got)
	}
	if got := tr.VocalMs(); got != 15000 {
		t.Fatalf("vocal = %d, want 15000", got)
	}
	if got := tr.CueOutMs(); got != 175000 {
		t.Fatalf("cue out = %d, want 175000", got)
	}
	for _, m := range tr.Markers {
		if !m.Auto {
			t.Fatalf("marcador do analisador tem que nascer Auto: %+v", m)
		}
	}
}

// Vocal 0 = "não marcou" (sinal ambíguo, regra do operador): nenhum marcador
// `intro` é escrito, e o LOOP fica indisponível pra essa faixa.
func TestApplyAnalysisVocalZeroWritesNoIntro(t *testing.T) {
	tr := localTrack(t)
	if err := tr.ApplyAnalysis(Analysis{BPM: 120, CueInMs: 500}); err != nil {
		t.Fatalf("ApplyAnalysis: %v", err)
	}
	if got := tr.VocalMs(); got != 0 {
		t.Fatalf("vocal marcado sem sinal claro: %d", got)
	}
	for _, m := range tr.Markers {
		if m.Kind == MarkerIntro {
			t.Fatalf("intro não deveria existir: %+v", m)
		}
	}
}

// Cue fora do intervalo da faixa é artefato de arredondamento entre duas sondas
// do MESMO arquivo (ffprobe dá a duração, o analisador dá o cue). Descartar
// aquele cue é melhor que perder BPM e loudness da rodada inteira.
func TestApplyAnalysisDropsOutOfRangeCues(t *testing.T) {
	tr := localTrack(t) // 3min = 180000ms
	err := tr.ApplyAnalysis(Analysis{BPM: 120, CueInMs: 900, CueOutMs: 180001, VocalMs: 999999})
	if err != nil {
		t.Fatalf("ApplyAnalysis: %v", err)
	}
	if tr.BPM != 120 {
		t.Fatalf("bpm perdido junto com o cue inválido: %v", tr.BPM)
	}
	if got := tr.CueOutMs(); got != 0 {
		t.Fatalf("cue out fora da faixa deveria ser descartado, veio %d", got)
	}
	if got := tr.VocalMs(); got != 0 {
		t.Fatalf("vocal fora da faixa deveria ser descartado, veio %d", got)
	}
	if got := tr.CueInMs(); got != 900 {
		t.Fatalf("cue in válido foi descartado junto: %d", got)
	}
}

// start ≤ end é invariante do SetMarkers: um par invertido rejeitaria o conjunto
// TODO, então o fim é descartado sozinho em vez de derrubar a análise.
func TestApplyAnalysisDropsInvertedCueOut(t *testing.T) {
	tr := localTrack(t)
	if err := tr.ApplyAnalysis(Analysis{BPM: 120, CueInMs: 50000, CueOutMs: 1000}); err != nil {
		t.Fatalf("ApplyAnalysis: %v", err)
	}
	if got := tr.CueOutMs(); got != 0 {
		t.Fatalf("cue out invertido deveria sumir, veio %d", got)
	}
	if got := tr.CueInMs(); got != 50000 {
		t.Fatalf("cue in perdido: %d", got)
	}
}

// Re-análise substitui as marcas do analisador e PRESERVA a do operador.
func TestApplyAnalysisPreservesManualMarker(t *testing.T) {
	tr := localTrack(t)
	if err := tr.SetMarkers([]Marker{{Kind: MarkerStart, PosMs: 4000, Label: "meu início"}}); err != nil {
		t.Fatalf("SetMarkers: %v", err)
	}
	if err := tr.ApplyAnalysis(Analysis{BPM: 120, CueInMs: 900, VocalMs: 20000}); err != nil {
		t.Fatalf("ApplyAnalysis: %v", err)
	}
	if got := tr.CueInMs(); got != 4000 {
		t.Fatalf("o analisador sobrescreveu o start do operador: %d", got)
	}
	if got := tr.VocalMs(); got != 20000 {
		t.Fatalf("vocal automático não entrou: %d", got)
	}
}

// ApplyAutoMarkers propaga o erro do SetMarkers (não engole validação).
func TestApplyAutoMarkersInvalidKind(t *testing.T) {
	tr := localTrack(t)
	err := tr.ApplyAutoMarkers([]Marker{{Kind: MarkerKind("nope"), PosMs: 10}})
	if !errors.Is(err, ErrBadMarkerKind) {
		t.Fatalf("want ErrBadMarkerKind, got %v", err)
	}
}

// Faixa não-PCM não tem marcador nenhum — nem automático.
func TestApplyAutoMarkersRejectsNonPCM(t *testing.T) {
	tr, err := NewTrack("sp2", shared.SourceSpotify, "DRM", 3*time.Minute)
	if err != nil {
		t.Fatalf("NewTrack: %v", err)
	}
	tr.SpotifyURI = "spotify:track:abc"
	if err := tr.ApplyAutoMarkers(nil); !errors.Is(err, ErrMarkersNotPCM) {
		t.Fatalf("want ErrMarkersNotPCM, got %v", err)
	}
}

// Duas marcas AUTO do mesmo kind singleton: a segunda é descartada em vez de
// derrubar o conjunto por ErrDuplicateMarker.
func TestApplyAutoMarkersDedupesSingleton(t *testing.T) {
	tr := localTrack(t)
	err := tr.ApplyAutoMarkers([]Marker{
		{Kind: MarkerStart, PosMs: 1000},
		{Kind: MarkerStart, PosMs: 2000},
	})
	if err != nil {
		t.Fatalf("ApplyAutoMarkers: %v", err)
	}
	if got := tr.CueInMs(); got != 1000 {
		t.Fatalf("primeira marca deveria vencer, veio %d", got)
	}
	if len(tr.Markers) != 1 {
		t.Fatalf("want 1 marker, got %d: %+v", len(tr.Markers), tr.Markers)
	}
}

// Kind repetível (hook/stamp) não é deduplicado — o analisador pode marcar
// vários refrões sem que um apague o outro.
func TestApplyAutoMarkersKeepsRepeatableKinds(t *testing.T) {
	tr := localTrack(t)
	err := tr.ApplyAutoMarkers([]Marker{
		{Kind: MarkerHook, PosMs: 30000},
		{Kind: MarkerHook, PosMs: 90000},
	})
	if err != nil {
		t.Fatalf("ApplyAutoMarkers: %v", err)
	}
	if len(tr.Markers) != 2 {
		t.Fatalf("want 2 hooks, got %+v", tr.Markers)
	}
}

// Re-análise MOVE a marca automática anterior em vez de empilhar duas — é o que
// o flag Auto existe pra permitir. Sem isto, a segunda rodada bateria em
// ErrDuplicateMarker e a faixa ficaria presa no cue velho pra sempre.
func TestApplyAutoMarkersReplacesPreviousAuto(t *testing.T) {
	tr := localTrack(t)
	if err := tr.ApplyAutoMarkers([]Marker{{Kind: MarkerStart, PosMs: 1000}}); err != nil {
		t.Fatalf("1ª rodada: %v", err)
	}
	if err := tr.ApplyAutoMarkers([]Marker{{Kind: MarkerStart, PosMs: 2500}}); err != nil {
		t.Fatalf("2ª rodada: %v", err)
	}
	if len(tr.Markers) != 1 {
		t.Fatalf("marca automática empilhou: %+v", tr.Markers)
	}
	if got := tr.CueInMs(); got != 2500 {
		t.Fatalf("cue não foi movido: %d", got)
	}
}

// Marcador manual JÁ inválido (gravado antes de a duração ser corrigida pra
// menos, por exemplo) faz o conjunto inteiro ser rejeitado — e o erro sobe, não
// é engolido. O operador precisa saber que aquela faixa não aceita re-análise
// enquanto a marca dele estiver fora do intervalo.
func TestApplyAnalysisSurfacesMarkerError(t *testing.T) {
	tr := localTrack(t) // 3min
	// atribuição direta: burla o SetMarkers de propósito, simulando dado gravado
	// por uma versão anterior com outra duração.
	tr.Markers = []Marker{{Kind: MarkerHook, PosMs: 999999}}
	err := tr.ApplyAnalysis(Analysis{BPM: 120, CueInMs: 500})
	if !errors.Is(err, ErrMarkerOutOfRange) {
		t.Fatalf("want ErrMarkerOutOfRange, got %v", err)
	}
}

// Acessor de cue sem marcador nenhum devolve 0 = "sem corte / sem marca".
func TestCueAccessorsDefaultZero(t *testing.T) {
	tr := localTrack(t)
	if tr.CueInMs() != 0 || tr.CueOutMs() != 0 || tr.VocalMs() != 0 {
		t.Fatalf("faixa sem marcador deveria devolver zeros: %+v", tr.Markers)
	}
}

func TestApplyAnalysisValidateError(t *testing.T) {
	// PCM track missing its path fails Validate inside ApplyAnalysis.
	tr, err := NewTrack("id2", shared.SourceLocal, "No Path", 3*time.Minute)
	if err != nil {
		t.Fatalf("NewTrack: %v", err)
	}
	if err := tr.ApplyAnalysis(Analysis{BPM: 100}); !errors.Is(err, ErrLocalNeedsPath) {
		t.Fatalf("want ErrLocalNeedsPath from Validate, got %v", err)
	}
}
