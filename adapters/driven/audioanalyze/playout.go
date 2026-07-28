package audioanalyze

import (
	"math"
	"strconv"
	"strings"
)

/* Sinais de PLAYOUT extraídos de UMA passada de ffmpeg: onde o áudio começa e
   termina de fato (trim), quão alto o master é (loudness) e onde a voz entra
   (vocal). Este arquivo guarda só as funções PURAS — nada aqui executa processo.

   É de propósito: em produção a imagem tem ffmpeg, mas o CI não roda binário
   nenhum. Espelha o `bpmFromBeats`, que já é testado sem o `aubio` instalado.
   As fixtures em testdata/ são saída REAL do ffmpeg (não texto inventado), então
   uma mudança de formato do upstream aparece como teste vermelho, não como
   campo zerado em silêncio na produção. */

// silenceSpan é um trecho silencioso detectado pelo `silencedetect`, em ms.
// endMs < 0 significa "o arquivo acabou dentro do silêncio" (o filtro não chegou
// a fechar o intervalo) — acontece em algumas versões do ffmpeg.
type silenceSpan struct {
	startMs int64
	endMs   int64
}

// parseSilence lê as linhas `silence_start:` / `silence_end:` do log e devolve os
// cues de reprodução em ms:
//
//   - cueIn  = fim do silêncio que começa em 0 (o vazio da frente). 0 = o arquivo
//     já começa com áudio, nada a cortar.
//   - cueOut = início do último silêncio que vai até o fim do arquivo (o vazio do
//     final). 0 = o arquivo termina com áudio.
//
// durMs é a duração real (do ffprobe) e serve pra decidir o que é "até o fim":
// **o ffmpeg FECHA o silêncio final no EOF** (emite `silence_end` com o tempo
// total), então esperar um `silence_start` órfão não funciona — o teste com saída
// real mostrou o par completo. A regra é geométrica: o silêncio que TERMINA no
// fim do arquivo é o vazio do final, tenha ele `silence_end` ou não.
func parseSilence(raw string, durMs int64) (cueIn, cueOut int64) {
	var spans []silenceSpan
	open := -1
	for _, line := range strings.Split(raw, "\n") {
		if v, ok := afterToken(line, "silence_start:"); ok {
			if open >= 0 {
				// start sem end anterior: fecha o aberto como indefinido.
				spans = append(spans, silenceSpan{startMs: int64(open), endMs: -1})
			}
			open = int(secToMs(v))
			continue
		}
		if v, ok := afterToken(line, "silence_end:"); ok {
			if open < 0 {
				continue // end sem start: linha truncada/log parcial
			}
			spans = append(spans, silenceSpan{startMs: int64(open), endMs: secToMs(v)})
			open = -1
		}
	}
	if open >= 0 {
		spans = append(spans, silenceSpan{startMs: int64(open), endMs: -1})
	}
	if len(spans) == 0 {
		return 0, 0
	}
	// tolerância: o `silence_end` do EOF costuma bater a duração com alguns ms de
	// diferença (o filtro trabalha por bloco de amostras, não por milissegundo).
	const edgeMs = 120
	first := spans[0]
	if first.startMs <= edgeMs && first.endMs > 0 {
		cueIn = first.endMs
	}
	last := spans[len(spans)-1]
	atEnd := last.endMs < 0 || (durMs > 0 && last.endMs >= durMs-edgeMs)
	if atEnd && last.startMs > cueIn {
		cueOut = last.startMs
	}
	return cueIn, cueOut
}

// parseIntegratedLUFS lê o loudness integrado (EBU R128) do resumo do `ebur128`.
//
// Só a linha do RESUMO conta: com `framelog` ligado o filtro também imprime um
// `I: … LUFS` por quadro, e pegar o primeiro daria o valor do primeiro 100ms
// (tipicamente −70, o silêncio da entrada) em vez do da faixa inteira. O resumo
// é a única linha cujo conteúdo, sem espaços, é EXATAMENTE `I: <n> LUFS` — as de
// quadro carregam `t:`, `M:`, `S:`, `LRA:` na mesma linha.
//
// Devolve 0 quando não há medição (nenhum resumo, ou `-inf` de faixa muda);
// 0 significa "sem medida" em toda a cadeia — o playout não corrige o ganho.
func parseIntegratedLUFS(raw string) float64 {
	for _, line := range strings.Split(raw, "\n") {
		f := strings.Fields(strings.TrimSpace(stripFilterTag(line)))
		if len(f) != 3 || f[0] != "I:" || f[2] != "LUFS" {
			continue
		}
		v, err := strconv.ParseFloat(f[1], 64)
		if err != nil || math.IsInf(v, 0) || math.IsNaN(v) {
			return 0
		}
		return v
	}
	return 0
}

// rmsBucket é a energia mid e side de uma janela de 200ms, em dB.
// posMs é o início da janela.
type rmsBucket struct {
	posMs int64
	mid   float64
	side  float64
}

// negInf marca um bucket sem energia (`-inf` do astats). Vale como "silêncio",
// nunca como número — somar −inf a uma média envenenaria a linha de base.
var negInf = math.Inf(-1)

// parseChannelRMS lê a saída do `ametadata=print` (canal 1 = mid, canal 2 = side)
// e devolve um bucket por janela. Uma janela só entra completa: se faltar um dos
// canais (log truncado), ela é descartada — meia leitura mentiria sobre a
// centralização, que é justamente o discriminador de voz.
func parseChannelRMS(raw string) []rmsBucket {
	var out []rmsBucket
	var cur rmsBucket
	haveFrame, haveMid, haveSide := false, false, false
	flush := func() {
		if haveFrame && haveMid && haveSide {
			out = append(out, cur)
		}
	}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if v, ok := afterToken(line, "pts_time:"); ok {
			flush()
			cur = rmsBucket{posMs: secToMs(v)}
			haveFrame, haveMid, haveSide = true, false, false
			continue
		}
		if v, ok := valueAfter(line, "lavfi.astats.1.RMS_level="); ok {
			cur.mid, haveMid = v, true
			continue
		}
		if v, ok := valueAfter(line, "lavfi.astats.2.RMS_level="); ok {
			cur.side, haveSide = v, true
		}
	}
	flush()
	return out
}

// Parâmetros do detector de vocal. Todos existem por causa da regra do operador:
// **na dúvida, não marca**. Um vocal marcado no lugar errado faz a locução entrar
// por cima do canto — pior que não ter marca nenhuma.
const (
	// bucketMs é a janela do astats (asetnsamples=8820 @ 44,1kHz).
	bucketMs = 200
	// vocalMinMs: nada é marcado no primeiro segundo. Faixa que já entra cantando
	// não precisa de marca (não há base instrumental pra usar de loop), e o
	// arranque do arquivo é onde os filtros ainda estão assentando.
	vocalMinMs = 1000
	// vocalHoldMs: a subida tem que SUSTENTAR. Um golpe de caixa ou um prato
	// centralizado levanta o mid por uma janela; voz fica.
	vocalHoldMs = 1500
	// vocalRiseDB: quanto o "quão centralizado" precisa subir acima da linha de
	// base do intro pra contar como voz.
	vocalRiseDB = 6.0
	// vocalFloorDB: bucket abaixo disto é silêncio/ruído de fundo — não entra na
	// linha de base nem dispara detecção.
	vocalFloorDB = -60.0
	// vocalBaselineMin: sem pelo menos isto de intro instrumental medido não há
	// linha de base confiável ⇒ não marca (a regra do operador de novo).
	vocalBaselineMin = 5
)

// detectVocalStart devolve, em ms, onde a voz entra — ou 0 quando o sinal não é
// claro o bastante, que é a resposta correta na maioria das faixas duvidosas.
//
// A ideia física: voz de estúdio é MIXADA NO CENTRO; instrumento é panorâmico.
// Somar os canais (mid) reforça o que é central; subtrair (side) o cancela. Logo
// `mid − side` é uma medida de "quão centralizado está este trecho". O bandpass
// em ~1,4kHz (aplicado antes, no ffmpeg) tira bumbo e baixo — que também são
// centrais — da conta, deixando a região da voz.
//
// O algoritmo: mede a linha de base de `mid − side` no intro (depois do cueIn,
// antes de qualquer subida) e procura a primeira subida de pelo menos
// vocalRiseDB que se SUSTENTE por vocalHoldMs. Mono não tem side (mid − side é
// constante) ⇒ nenhuma subida ⇒ devolve 0, exatamente como se quer.
func detectVocalStart(buckets []rmsBucket, cueInMs int64) int64 {
	var samples []centerSample
	for _, b := range buckets {
		if b.posMs < cueInMs || b.posMs < vocalMinMs {
			continue
		}
		if math.IsInf(b.mid, 0) || b.mid < vocalFloorDB {
			continue // silêncio: não é intro nem voz
		}
		// side em -inf = nada fora do centro. Isso É centralização máxima, mas
		// -inf não é número: o piso entra como valor pra manter a conta finita.
		side := b.side
		if math.IsInf(side, 0) || side < vocalFloorDB {
			side = vocalFloorDB
		}
		samples = append(samples, centerSample{posMs: b.posMs, center: b.mid - side})
	}
	if len(samples) < vocalBaselineMin {
		return 0 // intro curto/ausente: sem linha de base, não marca
	}
	// Linha de base = MEDIANA das primeiras janelas do intro. Mediana e não média
	// porque um único estouro no começo (contagem, efeito) deslocaria a média e
	// esconderia a entrada real da voz.
	base := median(centers(samples[:vocalBaselineMin]))
	need := int(vocalHoldMs / bucketMs)
	for i := range samples {
		if samples[i].center < base+vocalRiseDB {
			continue
		}
		if i+need > len(samples) {
			return 0 // subiu, mas o arquivo acaba antes de provar que sustenta
		}
		sustained := true
		for j := i; j < i+need; j++ {
			if samples[j].center < base+vocalRiseDB {
				sustained = false
				break
			}
		}
		if sustained {
			return samples[i].posMs
		}
	}
	return 0
}

// centerSample é "quão centralizado" (mid − side, em dB) está um instante da
// faixa. É o único número que o detector de vocal olha.
type centerSample struct {
	posMs  int64
	center float64
}

func centers(ss []centerSample) []float64 {
	out := make([]float64, len(ss))
	for i, s := range ss {
		out[i] = s.center
	}
	return out
}

// median devolve a mediana de vs (a fatia é copiada — a ordem do chamador
// importa: ela representa o tempo).
func median(vs []float64) float64 {
	if len(vs) == 0 {
		return 0
	}
	cp := append([]float64(nil), vs...)
	for i := 1; i < len(cp); i++ {
		for j := i; j > 0 && cp[j] < cp[j-1]; j-- {
			cp[j], cp[j-1] = cp[j-1], cp[j]
		}
	}
	return cp[len(cp)/2]
}

// afterToken devolve o número que vem logo depois de token na linha.
func afterToken(line, token string) (float64, bool) {
	i := strings.Index(line, token)
	if i < 0 {
		return 0, false
	}
	rest := strings.TrimSpace(line[i+len(token):])
	f := strings.Fields(rest)
	if len(f) == 0 {
		return 0, false
	}
	v, err := strconv.ParseFloat(f[0], 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// valueAfter devolve o float de uma linha `chave=valor`. `-inf` (astats em
// silêncio) é um valor legítimo e volta como math.Inf(-1).
func valueAfter(line, prefix string) (float64, bool) {
	if !strings.HasPrefix(line, prefix) {
		return 0, false
	}
	raw := strings.TrimSpace(strings.TrimPrefix(line, prefix))
	switch raw {
	case "-inf", "-Inf":
		return negInf, true
	case "inf", "Inf", "nan":
		return 0, false
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// stripFilterTag remove o prefixo `[Parsed_xxx @ 0x…]` que o ffmpeg põe em cada
// linha de log de filtro, pra comparação de campos funcionar.
func stripFilterTag(line string) string {
	if i := strings.Index(line, "] "); i >= 0 && strings.HasPrefix(strings.TrimSpace(line), "[") {
		return line[i+2:]
	}
	return line
}

// secToMs converte segundos (float do ffmpeg) em ms, arredondando.
func secToMs(sec float64) int64 {
	if sec < 0 {
		return 0
	}
	return int64(math.Round(sec * 1000))
}
