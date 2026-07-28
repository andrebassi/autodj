package audioanalyze

import (
	"context"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

)

/* As fixtures em testdata/ são saída REAL do ffmpeg 8.x sobre um arquivo
   sintético de 12s montado assim:

     0–2s    silêncio
     2–10s   instrumento (senoide 220Hz) HARD-LEFT
     5–10s   voz simulada (senoide 1400Hz) CENTRALIZADA, por cima
     10–12s  silêncio

   Ou seja, a resposta certa é conhecida por construção: cueIn=2000,
   cueOut=10000, vocal=5000. Testar contra saída real (e não texto inventado)
   é o que faz uma mudança de formato do upstream aparecer como teste vermelho
   em vez de campo zerado em silêncio na produção. */

func fixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("fixture %s: %v", name, err)
	}
	return string(b)
}

func TestParseSilenceRealOutput(t *testing.T) {
	in, out := parseSilence(fixture(t, "ffmpeg_log.txt"), 12000)
	if in != 2000 {
		t.Fatalf("cueIn = %d, want 2000", in)
	}
	if out != 10000 {
		t.Fatalf("cueOut = %d, want 10000", out)
	}
}

func TestParseSilence(t *testing.T) {
	cases := []struct {
		name          string
		raw           string
		durMs         int64
		wantIn, wantO int64
	}{
		{"sem silêncio nenhum", "nada aqui", 180000, 0, 0},
		{
			// só vazio na frente: a faixa termina tocando.
			name:   "só cue in",
			raw:    "silence_start: 0\nsilence_end: 1.5 | silence_duration: 1.5\n",
			durMs:  180000,
			wantIn: 1500,
		},
		{
			// silêncio no MEIO da faixa (pausa dramática) não é trim: não começa em
			// 0 nem termina no fim do arquivo.
			name:  "silêncio no meio é ignorado",
			raw:   "silence_start: 40\nsilence_end: 42 | silence_duration: 2\n",
			durMs: 180000,
		},
		{
			// versão de ffmpeg que NÃO fecha o silêncio final no EOF.
			name:   "start final órfão vira cue out",
			raw:    "silence_start: 0\nsilence_end: 2\nsilence_start: 170\n",
			durMs:  180000,
			wantIn: 2000,
			wantO:  170000,
		},
		{
			// dois starts seguidos (log intercalado): o primeiro fecha indefinido.
			name:  "start repetido não derruba o parse",
			raw:   "silence_start: 5\nsilence_start: 170\nsilence_end: 180\n",
			durMs: 180000,
			wantO: 170000,
		},
		{
			// end sem start (log truncado no começo) é descartado.
			name:  "end órfão ignorado",
			raw:   "silence_end: 3 | silence_duration: 3\n",
			durMs: 180000,
		},
		{
			// duração desconhecida (ffprobe ausente): sem régua pra dizer o que é
			// "até o fim", só um start órfão conta.
			name:   "sem duração, só órfão conta",
			raw:    "silence_start: 0\nsilence_end: 1\nsilence_start: 170\nsilence_end: 175\n",
			durMs:  0,
			wantIn: 1000,
		},
		{
			// arquivo 100% mudo: um silêncio só, do 0 ao fim. Cortar tudo deixaria
			// a faixa sem nada — cueOut tem de ficar 0 (não cortar).
			name:   "faixa inteiramente muda",
			raw:    "silence_start: 0\nsilence_end: 180 | silence_duration: 180\n",
			durMs:  180000,
			wantIn: 180000,
		},
		{"linha malformada", "silence_start: abc\n", 180000, 0, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in, out := parseSilence(c.raw, c.durMs)
			if in != c.wantIn || out != c.wantO {
				t.Fatalf("parseSilence = (%d,%d), want (%d,%d)", in, out, c.wantIn, c.wantO)
			}
		})
	}
}

func TestParseIntegratedLUFSRealOutput(t *testing.T) {
	if got := parseIntegratedLUFS(fixture(t, "ffmpeg_log.txt")); got != -24.3 {
		t.Fatalf("LUFS = %v, want -24.3", got)
	}
}

func TestParseIntegratedLUFS(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want float64
	}{
		{"resumo simples", "  Integrated loudness:\n    I:         -13.4 LUFS\n", -13.4},
		{"sem resumo", "nada de loudness aqui", 0},
		// A linha POR QUADRO carrega o mesmo `I: … LUFS`. Pegá-la daria o valor do
		// primeiro 100ms (o silêncio de entrada, ≈ -70) em vez do da faixa inteira.
		{
			name: "linha de quadro não engana",
			raw: "[Parsed_ebur128_2 @ 0x1] t: 0.09  TARGET:-23 LUFS    M:-120.7 S:-120.7     I: -70.0 LUFS       LRA: 0.0 LU\n" +
				"  Integrated loudness:\n    I:         -9.9 LUFS\n",
			want: -9.9,
		},
		{"faixa muda devolve zero", "    I:         -inf LUFS\n", 0},
		{"valor não numérico", "    I:         abc LUFS\n", 0},
		{"positivo (clip absurdo) passa", "    I:         2.5 LUFS\n", 2.5},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := parseIntegratedLUFS(c.raw); got != c.want {
				t.Fatalf("parseIntegratedLUFS = %v, want %v", got, c.want)
			}
		})
	}
}

func TestParseChannelRMSRealOutput(t *testing.T) {
	bs := parseChannelRMS(fixture(t, "astats_midside.txt"))
	if len(bs) != 60 {
		t.Fatalf("want 60 janelas de 200ms em 12s, got %d", len(bs))
	}
	if bs[0].posMs != 0 || bs[1].posMs != 200 {
		t.Fatalf("posições erradas: %+v %+v", bs[0], bs[1])
	}
	// 0–2s é silêncio: astats reporta -inf nos dois canais.
	if !math.IsInf(bs[0].mid, -1) || !math.IsInf(bs[0].side, -1) {
		t.Fatalf("silêncio deveria ser -inf: %+v", bs[0])
	}
	// Em 3,0s só o instrumento hard-left toca ⇒ mid e side IGUAIS (diferença 0).
	inst := bucketAt(t, bs, 3000)
	if d := inst.mid - inst.side; math.Abs(d) > 0.5 {
		t.Fatalf("instrumento panorâmico deveria dar mid−side≈0, deu %.2f", d)
	}
	// Em 6,0s a voz centralizada está por cima ⇒ mid MUITO acima do side.
	voc := bucketAt(t, bs, 6000)
	if d := voc.mid - voc.side; d < 10 {
		t.Fatalf("voz centralizada deveria levantar mid−side, deu %.2f", d)
	}
}

func bucketAt(t *testing.T, bs []rmsBucket, posMs int64) rmsBucket {
	t.Helper()
	for _, b := range bs {
		if b.posMs == posMs {
			return b
		}
	}
	t.Fatalf("sem bucket em %dms", posMs)
	return rmsBucket{}
}

func TestParseChannelRMS(t *testing.T) {
	t.Run("janela incompleta é descartada", func(t *testing.T) {
		// segunda janela só tem o canal 1: meia leitura mentiria sobre a
		// centralização, que é o discriminador de voz.
		raw := "frame:0 pts_time:0\nlavfi.astats.1.RMS_level=-20\nlavfi.astats.2.RMS_level=-30\n" +
			"frame:1 pts_time:0.2\nlavfi.astats.1.RMS_level=-21\n"
		bs := parseChannelRMS(raw)
		if len(bs) != 1 {
			t.Fatalf("want 1 bucket, got %d: %+v", len(bs), bs)
		}
	})
	t.Run("valor ilegível não vira bucket", func(t *testing.T) {
		raw := "frame:0 pts_time:0\nlavfi.astats.1.RMS_level=xx\nlavfi.astats.2.RMS_level=-30\n"
		if bs := parseChannelRMS(raw); len(bs) != 0 {
			t.Fatalf("want 0 buckets, got %+v", bs)
		}
	})
	t.Run("vazio", func(t *testing.T) {
		if bs := parseChannelRMS(""); len(bs) != 0 {
			t.Fatalf("want 0 buckets, got %+v", bs)
		}
	})
}

func TestDetectVocalStartRealOutput(t *testing.T) {
	bs := parseChannelRMS(fixture(t, "astats_midside.txt"))
	got := detectVocalStart(bs, 2000)
	if got != 5000 {
		t.Fatalf("vocal = %d, want 5000", got)
	}
}

// mkBuckets monta uma sequência sintética de janelas de 200ms: `n` janelas com
// (mid, side) constantes a partir de startMs.
func mkBuckets(startMs int64, n int, mid, side float64) []rmsBucket {
	out := make([]rmsBucket, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, rmsBucket{posMs: startMs + int64(i)*bucketMs, mid: mid, side: side})
	}
	return out
}

func TestDetectVocalStart(t *testing.T) {
	t.Run("mono não marca", func(t *testing.T) {
		// mono vira mid==side em toda janela: não há discriminador, e marcar seria
		// chutar. A regra do operador é não marcar.
		bs := mkBuckets(0, 60, -20, -20)
		if got := detectVocalStart(bs, 0); got != 0 {
			t.Fatalf("mono não pode marcar vocal, marcou em %d", got)
		}
	})
	t.Run("subida que não sustenta é ignorada", func(t *testing.T) {
		// um golpe centralizado (prato, caixa) levanta o mid por 1 janela só.
		bs := mkBuckets(0, 30, -30, -30)
		bs = append(bs, rmsBucket{posMs: 6000, mid: -10, side: -30}) // pico isolado
		bs = append(bs, mkBuckets(6200, 20, -30, -30)...)
		if got := detectVocalStart(bs, 0); got != 0 {
			t.Fatalf("pico isolado não é voz, marcou em %d", got)
		}
	})
	t.Run("subida sustentada marca", func(t *testing.T) {
		bs := mkBuckets(0, 30, -30, -30)                  // intro instrumental: 0 dB de centro
		bs = append(bs, mkBuckets(6000, 30, -10, -30)...) // voz: +20 dB de centro
		if got := detectVocalStart(bs, 0); got != 6000 {
			t.Fatalf("vocal = %d, want 6000", got)
		}
	})
	t.Run("nada antes de 1s", func(t *testing.T) {
		// faixa que já entra cantando: não há base instrumental pra usar de loop,
		// então a marca não serviria pra nada.
		bs := mkBuckets(0, 60, -10, -30)
		if got := detectVocalStart(bs, 0); got != 0 && got < vocalMinMs {
			t.Fatalf("marcou antes de %dms: %d", vocalMinMs, got)
		}
	})
	t.Run("intro curto demais não marca", func(t *testing.T) {
		// sem janelas suficientes não há linha de base confiável.
		bs := mkBuckets(2000, 3, -20, -40)
		if got := detectVocalStart(bs, 2000); got != 0 {
			t.Fatalf("sem linha de base não pode marcar, marcou em %d", got)
		}
	})
	t.Run("sobe no fim sem tempo de provar", func(t *testing.T) {
		bs := mkBuckets(0, 30, -30, -30)
		bs = append(bs, mkBuckets(6000, 3, -10, -30)...) // só 600ms, < vocalHoldMs
		if got := detectVocalStart(bs, 0); got != 0 {
			t.Fatalf("subida sem sustentação não conta, marcou em %d", got)
		}
	})
	t.Run("silêncio não entra na linha de base", func(t *testing.T) {
		// buckets em -inf (antes do cueIn) são descartados; se entrassem, a
		// mediana desabaria e qualquer coisa pareceria "subida".
		bs := mkBuckets(0, 10, negInf, negInf)
		bs = append(bs, mkBuckets(2000, 30, -30, -30)...)
		bs = append(bs, mkBuckets(8000, 30, -10, -30)...)
		if got := detectVocalStart(bs, 2000); got != 8000 {
			t.Fatalf("vocal = %d, want 8000", got)
		}
	})
	t.Run("side em -inf vira o piso", func(t *testing.T) {
		// centralização perfeita: side sem energia nenhuma. Sem o piso a conta
		// viraria +inf e envenenaria a mediana.
		bs := mkBuckets(0, 30, -30, negInf)
		if got := detectVocalStart(bs, 0); got != 0 {
			t.Fatalf("centro constante não é entrada de voz, marcou em %d", got)
		}
	})
	t.Run("lista vazia", func(t *testing.T) {
		if got := detectVocalStart(nil, 0); got != 0 {
			t.Fatalf("want 0, got %d", got)
		}
	})
}

func TestMedian(t *testing.T) {
	if got := median(nil); got != 0 {
		t.Fatalf("median(nil) = %v, want 0", got)
	}
	if got := median([]float64{5, 1, 3}); got != 3 {
		t.Fatalf("median = %v, want 3", got)
	}
	// A ORDEM DO CHAMADOR representa o tempo: median não pode reordenar no lugar.
	src := []float64{5, 1, 3}
	_ = median(src)
	if src[0] != 5 {
		t.Fatalf("median embaralhou a fatia do chamador: %+v", src)
	}
}

func TestSecToMs(t *testing.T) {
	if got := secToMs(1.2345); got != 1235 {
		t.Fatalf("secToMs = %d, want 1235 (arredonda, não trunca)", got)
	}
	if got := secToMs(-1); got != 0 {
		t.Fatalf("negativo deveria virar 0, veio %d", got)
	}
}

func TestStripFilterTag(t *testing.T) {
	if got := stripFilterTag("[Parsed_x @ 0xff] I: -9 LUFS"); got != "I: -9 LUFS" {
		t.Fatalf("got %q", got)
	}
	if got := stripFilterTag("sem tag"); got != "sem tag" {
		t.Fatalf("got %q", got)
	}
}

func TestValueAfter(t *testing.T) {
	if v, ok := valueAfter("lavfi.astats.1.RMS_level=-inf", "lavfi.astats.1.RMS_level="); !ok || !math.IsInf(v, -1) {
		t.Fatalf("got %v %v", v, ok)
	}
	if _, ok := valueAfter("lavfi.astats.1.RMS_level=nan", "lavfi.astats.1.RMS_level="); ok {
		t.Fatal("nan não é valor")
	}
	if _, ok := valueAfter("outra coisa", "lavfi."); ok {
		t.Fatal("prefixo diferente não deveria casar")
	}
}

func TestAfterToken(t *testing.T) {
	if _, ok := afterToken("nada", "silence_start:"); ok {
		t.Fatal("token ausente deveria falhar")
	}
	if _, ok := afterToken("silence_start:", "silence_start:"); ok {
		t.Fatal("token sem valor deveria falhar")
	}
}

// probe sem ffprobe (binário inexistente) devolve zeros — duração 0 desliga a
// regra de "silêncio até o fim", canais 0 desliga o vocal. Degradação, não erro.
func TestProbeAbsentBinary(t *testing.T) {
	c := &CLI{FFprobeBin: "nope-ffprobe-xyz"}
	dur, ch := c.probe(context.Background(), "/qualquer.wav")
	if dur != 0 || ch != 0 {
		t.Fatalf("want (0,0), got (%d,%d)", dur, ch)
	}
}

// A passada devolve erro quando o arquivo não existe — e o Analyze engole,
// deixando os campos zerados (análise parcial é resultado válido).
func TestPlayoutPassMissingFile(t *testing.T) {
	bin, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg não instalado")
	}
	if _, _, err := runPlayoutPass(context.Background(), bin, "/nao/existe.wav"); err == nil {
		t.Fatal("arquivo ausente deveria falhar")
	}
	c := New(t.TempDir())
	a, err := c.Analyze(context.Background(), "/nao/existe.wav")
	if err != nil {
		t.Fatalf("Analyze deveria degradar, não falhar: %v", err)
	}
	if a.CueInMs != 0 || a.LoudnessLUFS != 0 || a.VocalMs != 0 {
		t.Fatalf("campos deveriam ficar zerados: %+v", a)
	}
}

// Integração: a passada real sobre um arquivo construído com resposta conhecida.
// É o teste que prova que a cadeia inteira (ffmpeg → parsers → Analysis) casa —
// os unitários acima cobrem os parsers isoladamente.
func TestPlayoutEndToEnd(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg não instalado")
	}
	wav := filepath.Join(t.TempDir(), "sample.wav")
	gen := exec.Command("ffmpeg", "-y", "-loglevel", "error",
		"-f", "lavfi", "-i", "anullsrc=r=44100:cl=stereo:d=2",
		"-f", "lavfi", "-i", "sine=f=220:r=44100:d=8",
		"-f", "lavfi", "-i", "sine=f=1400:r=44100:d=5",
		"-f", "lavfi", "-i", "anullsrc=r=44100:cl=stereo:d=2",
		"-filter_complex", strings.Join([]string{
			"[1:a]pan=stereo|c0=1.0*c0|c1=0.0*c0,volume=0.5[inst]",
			"[2:a]adelay=3000|3000,pan=stereo|c0=0.7*c0|c1=0.7*c0,volume=0.6[voc]",
			"[inst][voc]amix=inputs=2:duration=first:normalize=0[mid]",
			"[0:a][mid][3:a]concat=n=3:v=0:a=1[out]",
		}, ";"),
		"-map", "[out]", "-c:a", "pcm_s16le", wav)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Skipf("geração do arquivo falhou: %v\n%s", err, out)
	}
	c := &CLI{FFmpegBin: "ffmpeg", FFprobeBin: "ffprobe"}
	a, err := c.Analyze(context.Background(), wav)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	near := func(got, want, tol int64) bool { return got >= want-tol && got <= want+tol }
	if !near(a.CueInMs, 2000, 150) {
		t.Fatalf("cueIn = %d, want ~2000", a.CueInMs)
	}
	if !near(a.CueOutMs, 10000, 150) {
		t.Fatalf("cueOut = %d, want ~10000", a.CueOutMs)
	}
	if !near(a.VocalMs, 5000, 400) {
		t.Fatalf("vocal = %d, want ~5000", a.VocalMs)
	}
	if a.LoudnessLUFS >= 0 || a.LoudnessLUFS < -60 {
		t.Fatalf("loudness fora do plausível: %v", a.LoudnessLUFS)
	}
}

/*
MONO não pode ganhar marca de vocal. A detecção é por mid/side, e num arquivo de
UM canal o `c1` do `pan` resolve como silêncio: `mid` e `side` saem numericamente
IGUAIS, então "quão centralizado" daria 0 em toda janela. Sem o gate por nº de
canais do ffprobe, o detector marcaria vocal em qualquer lugar — e vocal marcado
errado faz a locução entrar em cima do canto. Este teste tranca esse gate.
*/
func TestPlayoutMonoNaoMarcaVocal(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg não instalado")
	}
	wav := filepath.Join(t.TempDir(), "mono.wav")
	gen := exec.Command("ffmpeg", "-y", "-loglevel", "error",
		"-f", "lavfi", "-i", "sine=f=440:r=44100:d=6",
		"-af", "volume=0.4", "-ac", "1", "-c:a", "pcm_s16le", wav)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Skipf("geração do arquivo falhou: %v\n%s", err, out)
	}
	c := &CLI{FFmpegBin: "ffmpeg", FFprobeBin: "ffprobe"}
	a, err := c.Analyze(context.Background(), wav)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if a.VocalMs != 0 {
		t.Fatalf("mono não pode marcar vocal, marcou em %d", a.VocalMs)
	}
}

/*
BPM — o parser da saída do `aubio tempo`, e por que ele existe.

O código original mandava a saída de `aubio tempo` para `bpmFromBeats`, que espera
TIMESTAMPS de batida. Mas `aubio tempo` devolve "110.31 bpm" — um número só. Com
uma linha, `bpmFromBeats` não fecha dois deltas e devolve 0: o BPM ficava zerado
mesmo com a ferramenta instalada, sem erro nenhum. Medido no acervo do operador:
83 faixas, ZERO com BPM.
*/
func TestParseBPM(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want float64
	}{
		{"saída real do aubio tempo", "110.31 bpm\n", 110.3},
		{"maiúscula não muda nada", "128 BPM", 128},
		{"espaço extra", "  96.5   bpm  \n", 96.5},
		// sem a palavra "bpm" pode ser a saída do OUTRO binário (timestamps), e
		// tratá-la aqui daria um "BPM" que na verdade é o instante da 1ª batida.
		{"timestamps não são BPM", "1.665510\n2.215442\n", 0},
		{"vazio", "", 0},
		{"lixo", "erro: arquivo inválido", 0},
		// fora da faixa plausível: o aubio devolve 0 em trecho sem batida, e
		// gravar isso pareceria uma medição de verdade.
		{"zero é ausência de medida", "0 bpm", 0},
		{"rápido demais", "412 bpm", 0},
		{"lento demais", "12 bpm", 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := parseBPM(c.raw); got != c.want {
				t.Fatalf("parseBPM(%q) = %v, want %v", c.raw, got, c.want)
			}
		})
	}
}
