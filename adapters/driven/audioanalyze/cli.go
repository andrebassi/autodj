package audioanalyze

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"autodj/domain/library"
)

// CLI implements outbound.AudioAnalyzer using external tools. Each field names a
// binary looked up on PATH; an empty/absent tool is skipped so analysis
// degrades to whatever is installed. PeaksDir is where audiowaveform writes the
// peaks JSON that WaveformRef points at.
type CLI struct {
	AubioBin string // BPM via `aubio tempo` (default "aubio")
	// AubioTrackBin é o PLANO B do BPM: `aubiotrack` devolve os tempos das batidas
	// em vez do valor pronto. Existe porque o pacote do Debian (a imagem de deploy)
	// nem sempre traz o binário unificado — sem ele, prod ficaria sem BPM.
	AubioTrackBin string
	KeyBin        string // Camelot key detector (default "keyfinder-cli")
	WaveformBin   string // peaks generator (default "audiowaveform")
	PeaksDir      string // output dir for peaks JSON
	PixPerSecond  int    // audiowaveform resolution (default 20)

	// FFmpegBin/FFprobeBin produce the PLAYOUT hints (trim cues, loudness, vocal
	// entry) in a single decode. Unlike the three above, these two are ALREADY in
	// the production image — the others never were, which is why analysis silently
	// degraded to Noop in production and nothing was ever measured.
	FFmpegBin  string // default "ffmpeg"
	FFprobeBin string // default "ffprobe"
}

// New returns a CLI with default binary names. peaksDir defaults to
// ~/.autodj/peaks when empty.
func New(peaksDir string) *CLI {
	if peaksDir == "" {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			peaksDir = filepath.Join(home, ".autodj", "peaks")
		} else {
			peaksDir = "autodj-peaks"
		}
	}
	return &CLI{
		AubioBin:      "aubio",
		AubioTrackBin: "aubiotrack",
		KeyBin:        "keyfinder-cli",
		WaveformBin:   "audiowaveform",
		PeaksDir:      peaksDir,
		PixPerSecond:  20,
		FFmpegBin:     "ffmpeg",
		FFprobeBin:    "ffprobe",
	}
}

// Available reports whether at least one analysis tool is on PATH. Wiring uses
// it to pick CLI vs Noop.
func (c *CLI) Available() bool {
	for _, bin := range []string{c.AubioBin, c.AubioTrackBin, c.KeyBin, c.WaveformBin, c.FFmpegBin} {
		if bin == "" {
			continue
		}
		if _, err := exec.LookPath(bin); err == nil {
			return true
		}
	}
	return false
}

// Analyze runs whichever tools are present against path and assembles an
// Analysis. Individual tool failures are swallowed (that field stays zero); the
// method only errors on nothing — partial analysis is a valid result.
func (c *CLI) Analyze(ctx context.Context, path string) (library.Analysis, error) {
	var a library.Analysis
	// basta UM dos dois binários de BPM estar presente (ver CLI.bpm).
	if bin := resolve(c.AubioBin); bin != "" || resolve(c.AubioTrackBin) != "" {
		if bpm := c.bpm(ctx, bin, path); bpm > 0 {
			a.BPM = bpm
		}
	}
	if bin := resolve(c.KeyBin); bin != "" {
		a.MusicalKey = c.key(ctx, bin, path)
	}
	if bin := resolve(c.WaveformBin); bin != "" {
		a.WaveformRef = c.waveform(ctx, bin, path)
	}
	if bin := resolve(c.FFmpegBin); bin != "" {
		c.playout(ctx, bin, path, &a)
	}
	return a, nil
}

// playout preenche os sinais de reprodução (cues de trim, loudness e entrada do
// vocal) numa ÚNICA invocação do ffmpeg — um decode só para os três. Falhas são
// engolidas (os campos ficam zerados): análise parcial é resultado válido, é a
// mesma regra dos outros três binários.
func (c *CLI) playout(ctx context.Context, bin, path string, a *library.Analysis) {
	durMs, channels := c.probe(ctx, path)
	logs, meta, err := runPlayoutPass(ctx, bin, path)
	if err != nil {
		return
	}
	a.CueInMs, a.CueOutMs = parseSilence(logs, durMs)
	a.LoudnessLUFS = parseIntegratedLUFS(logs)
	// MONO NÃO TEM SIDE: sem a diferença entre os canais o discriminador de "está
	// no centro?" simplesmente não existe — todo trecho pareceria centralizado.
	// Pela regra do operador (na dúvida não marca), o vocal fica sem marca; trim e
	// loudness seguem normais, que é o que dá pra medir honestamente aqui.
	if channels < 2 {
		return
	}
	a.VocalMs = detectVocalStart(parseChannelRMS(meta), a.CueInMs)
}

// probe lê duração (ms) e nº de canais via ffprobe. Zeros quando indisponível —
// duração 0 desliga a regra de "silêncio até o fim" e canais 0 desliga o vocal.
func (c *CLI) probe(ctx context.Context, path string) (durMs int64, channels int) {
	bin := resolve(c.FFprobeBin)
	if bin == "" {
		return 0, 0
	}
	out, err := exec.CommandContext(ctx, bin,
		"-v", "error",
		"-select_streams", "a:0",
		"-show_entries", "stream=channels:format=duration",
		"-of", "default=noprint_wrappers=1:nokey=0",
		path,
	).Output()
	if err != nil {
		return 0, 0
	}
	for _, line := range strings.Split(string(out), "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		switch k {
		case "channels":
			if n, err := strconv.Atoi(v); err == nil {
				channels = n
			}
		case "duration":
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				durMs = secToMs(f)
			}
		}
	}
	return durMs, channels
}

/*
playoutFilter é a passada única. Três saídas de um decode só:

  - silencedetect → onde o áudio começa e termina de verdade (trim);

  - ebur128       → loudness integrado (ganho constante entre faixas);

  - mid/side      → onde a voz entra.

    O truque do vocal: voz de estúdio é mixada no CENTRO, instrumento é
    panorâmico. `mid` (L+R) reforça o central, `side` (L−R) o cancela — logo
    `mid − side` mede "quão centralizado". O `bandpass` em ~1,4kHz tira bumbo e
    baixo, que também são centrais, da conta. `amerge` junta os dois num stream de
    2 canais só pra que UM `astats` reporte os dois lados lado a lado.

    `framelog=quiet` é obrigatório: sem ele o ebur128 imprime uma linha por 100ms
    (≈1800 linhas numa faixa de 3min) e o resumo se perde no meio.
    `measure_perchannel=RMS_level` corta o astats de ~30 métricas por janela para
    uma — sem isso são 60 mil linhas numa faixa comum.
*/
const playoutFilter = `[0:a]asplit=3[a1][a2][a3];` +
	`[a1]silencedetect=noise=-45dB:d=0.3[s1];` +
	`[a2]ebur128=peak=true:framelog=quiet[s2];` +
	`[a3]asplit=2[m][sd];` +
	`[m]pan=mono|c0=0.5*c0+0.5*c1,bandpass=f=1400:width_type=o:w=2.5[mid];` +
	`[sd]pan=mono|c0=0.5*c0-0.5*c1,bandpass=f=1400:width_type=o:w=2.5[side];` +
	`[mid][side]amerge=inputs=2,asetnsamples=n=8820:p=0,` +
	`astats=metadata=1:reset=1:measure_perchannel=RMS_level:measure_overall=none,` +
	`ametadata=print:file=-[s3]`

// runPlayoutPass roda a passada e devolve (log do ffmpeg, metadados do astats).
//
// A separação é de graça e não é acidente: o `ametadata=print:file=-` escreve na
// SAÍDA PADRÃO, enquanto silencedetect e ebur128 logam na saída de ERRO. Dois
// canais, dois parsers, sem precisar desembaraçar texto.
func runPlayoutPass(ctx context.Context, bin, path string) (logs, meta string, err error) {
	cmd := exec.CommandContext(ctx, bin,
		"-hide_banner", "-nostats", "-loglevel", "info",
		"-i", path,
		"-filter_complex", playoutFilter,
		"-map", "[s1]", "-f", "null", "-",
		"-map", "[s2]", "-f", "null", "-",
		"-map", "[s3]", "-f", "null", "-",
	)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", "", err
	}
	return stderr.String(), stdout.String(), nil
}

// resolve returns the binary path if it exists on PATH, else "".
func resolve(bin string) string {
	if bin == "" {
		return ""
	}
	if p, err := exec.LookPath(bin); err == nil {
		return p
	}
	return ""
}

// bpm shells `aubio tempo`, which prints beat timestamps (seconds), and derives
// BPM from the median inter-beat interval — robust against outliers.
/*
bpm shella o aubio. DOIS binários, DOIS formatos de saída — e é por isso que o
recurso nunca funcionou até aqui.

  aubio tempo <arquivo>   → "110.31 bpm"        (UM número, já é o resultado)
  aubiotrack <arquivo>    → 1.665510\n2.215442… (timestamps de cada batida)

O código original chamava `aubio tempo` e mandava a saída para `bpmFromBeats`,
que espera timestamps: com uma linha só, ela não fecha dois deltas e devolve 0.
Ou seja, BPM ficava zerado MESMO com o aubio instalado, em silêncio.

Os dois nomes importam porque os dois ambientes divergem: o Homebrew instala
`aubio` (e também `aubiotrack`), enquanto o pacote `aubio-tools` do Debian — que
é o da imagem de deploy — traz `aubiotrack` sem garantia do unificado. Tentar os
dois é o que faz o mesmo código valer no Mac e no Linux.
*/
func (c *CLI) bpm(ctx context.Context, bin, path string) float64 {
	if bin != "" {
		if out, err := exec.CommandContext(ctx, bin, "tempo", path).Output(); err == nil {
			if v := parseBPM(string(out)); v > 0 {
				return v
			}
		}
	}
	// sem o unificado (ou saída inesperada): tenta o binário de batidas.
	if track := resolve(c.AubioTrackBin); track != "" {
		if out, err := exec.CommandContext(ctx, track, path).Output(); err == nil {
			return bpmFromBeats(string(out))
		}
	}
	return 0
}

/*
parseBPM lê a saída do `aubio tempo`: um número seguido da palavra "bpm".

Devolve 0 quando não reconhece — e o zero é o que faz o chamador tentar o outro
binário, em vez de gravar um valor inventado. Faixa de sanidade 20–300 porque o
aubio às vezes devolve 0 em trecho sem batida detectável, e um "BPM 0" gravado
seria pior que campo vazio: pareceria medido.
*/
func parseBPM(raw string) float64 {
	f := strings.Fields(raw)
	if len(f) < 2 || !strings.EqualFold(f[1], "bpm") {
		return 0
	}
	v, err := strconv.ParseFloat(f[0], 64)
	if err != nil || v < 20 || v > 300 {
		return 0
	}
	return float64(int(v*10+0.5)) / 10
}

// bpmFromBeats parses whitespace/newline-separated beat timestamps and returns
// 60 / median(delta). Exposed unexported for direct unit testing without the
// binary.
func bpmFromBeats(raw string) float64 {
	var beats []float64
	for _, line := range strings.Fields(raw) {
		if v, err := strconv.ParseFloat(strings.TrimSpace(line), 64); err == nil {
			beats = append(beats, v)
		}
	}
	if len(beats) < 2 {
		return 0
	}
	deltas := make([]float64, 0, len(beats)-1)
	for i := 1; i < len(beats); i++ {
		d := beats[i] - beats[i-1]
		if d > 0 {
			deltas = append(deltas, d)
		}
	}
	if len(deltas) == 0 {
		return 0
	}
	sort.Float64s(deltas)
	med := deltas[len(deltas)/2]
	if med <= 0 {
		return 0
	}
	bpm := 60.0 / med
	// Round to one decimal.
	return float64(int(bpm*10+0.5)) / 10
}

// key shells keyfinder-cli and returns the trimmed key string (Camelot when the
// build supports it).
func (c *CLI) key(ctx context.Context, bin, path string) string {
	out, err := exec.CommandContext(ctx, bin, path).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// waveform runs audiowaveform to write peaks JSON into PeaksDir and returns the
// output path as the WaveformRef. Returns "" on any failure.
func (c *CLI) waveform(ctx context.Context, bin, path string) string {
	if c.PeaksDir == "" {
		return ""
	}
	if err := os.MkdirAll(c.PeaksDir, 0o755); err != nil {
		return ""
	}
	pps := c.PixPerSecond
	if pps <= 0 {
		pps = 20
	}
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	out := filepath.Join(c.PeaksDir, base+".json")
	err := exec.CommandContext(ctx, bin,
		"-i", path,
		"-o", out,
		"--pixels-per-second", strconv.Itoa(pps),
		"-b", "8",
	).Run()
	if err != nil {
		return ""
	}
	return out
}
