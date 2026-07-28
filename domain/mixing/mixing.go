// Package mixing holds the station's fine-mixing settings: the per-media-type
// fade envelopes, ducking levels, silence-trim toggles and loudness
// normalization that shape how one item blends into the next. Pure value object
// — no external deps. JSON tags are the wire contract for GET/PUT /mix and
// mirror content.MixConfig (the seed default the operator screen ships), so the
// frontend carries a single mixing shape across both endpoints.
package mixing

import "errors"

// Settings is the whole fine-mixing configuration. Millisecond fade envelopes
// per media type (in/out), percentage ducking (overlap = media under a mix,
// talk = media under the mic), silence-trim toggles, an EBU-style loudness
// normalization target and the vocal-detection switch. It is a global playout
// config, not per-track — it never touches the two-engine matrix (Spotify stays
// non-mixable regardless).
//
// Trim/normalize/vocal só alcançam faixa LOCAL (PCM). Spotify e YouTube tocam
// out-of-band, sem PCM do lado de cá: não há o que medir nem o que cortar.
type Settings struct {
	MusicaIn     int  `json:"musicaIn"`
	MusicaOut    int  `json:"musicaOut"`
	VinhetaIn    int  `json:"vinhetaIn"`
	VinhetaOut   int  `json:"vinhetaOut"`
	ComercialIn  int  `json:"comercialIn"`
	ComercialOut int  `json:"comercialOut"`
	OverlapVol   int  `json:"overlapVol"`
	TalkVol      int  `json:"talkVol"`
	TrimMusicas  bool `json:"trimMusicas"`
	TrimVinhetas bool `json:"trimVinhetas"`
	Normalize    bool `json:"normalize"`
	NormalizeVol int  `json:"normalizeVol"`
	// DetectVocal liga a detecção de entrada do vocal na análise (marcador
	// `intro`). É o que habilita o LOOP na base instrumental enquanto se fala.
	DetectVocal bool `json:"detectVocal"`
	// AutoInstrumental manda preparar o INSTRUMENTAL (separação por IA) de toda
	// faixa local nova, assim que ela entra no acervo — em vez de o operador pedir
	// faixa a faixa pelo menu.
	//
	// DESLIGADO por padrão, e não é rigor: a separação leva ~1,4× o tempo real por
	// faixa, num trabalhador só. Um envio de 80 músicas viraria ~5 horas de CPU
	// moendo em segundo plano, disputando com o playout. Ligar isso é uma decisão
	// consciente de quem sabe que a máquina vai trabalhar a noite toda.
	AutoInstrumental bool `json:"autoInstrumental"`
}

// Faixa de alvos de normalização, em LUFS. NormalizeVol (0..100) é uma régua
// linear entre os dois: 0 ⇒ minTargetLUFS, 100 ⇒ maxTargetLUFS.
//
// Os extremos foram escolhidos para que o default HISTÓRICO (89) caia no padrão
// de streaming (≈ −13,3 LUFS) sem migrar um único dado: o campo já existia com
// esse valor, só não significava nada. Mais fundo que −24 é baixo demais para
// uso doméstico; acima de −12 já se aproxima do "loudness war" e passa a cortar
// a dinâmica que a normalização deveria preservar.
const (
	minTargetLUFS = -24.0
	maxTargetLUFS = -12.0
)

// TargetLUFS traduz o NormalizeVol (0..100) no alvo de loudness integrado.
// O playout compara o LUFS medido da faixa com este alvo e deriva o ganho —
// é aqui que "faixa masterizada alta e faixa baixa saem no mesmo volume" vira
// número. Fora de 0..100 (dado corrompido) o valor é clampado em vez de virar
// um alvo absurdo: um ganho errado é audível na hora, e silencioso é pior.
func (s Settings) TargetLUFS() float64 {
	v := s.NormalizeVol
	if v < 0 {
		v = 0
	}
	if v > 100 {
		v = 100
	}
	return minTargetLUFS + (float64(v)/100)*(maxTargetLUFS-minTargetLUFS)
}

// maxFadeMs caps a single fade at 30s — beyond that a "fade" is a whole track.
const maxFadeMs = 30000

// Validation sentinels.
var (
	// ErrNegativeFade is returned when any fade is below zero.
	ErrNegativeFade = errors.New("mixing: fade must be >= 0ms")
	// ErrFadeTooLong is returned when any fade exceeds maxFadeMs.
	ErrFadeTooLong = errors.New("mixing: fade must be <= 30000ms")
	// ErrVolPercent is returned when a ducking/normalize level is outside 0..100.
	ErrVolPercent = errors.New("mixing: volume must be within 0..100")
)

// Default returns the factory fine-mixing configuration. Must stay in sync with
// content.OperationalCatalog().Mix — guarded by a parity test in domain/content.
func Default() Settings {
	return Settings{
		MusicaIn: 500, MusicaOut: 2000,
		VinhetaIn: 50, VinhetaOut: 1000,
		ComercialIn: 50, ComercialOut: 1000,
		OverlapVol: 30, TalkVol: 20,
		TrimMusicas: true, TrimVinhetas: false,
		Normalize: true, NormalizeVol: 89,
		DetectVocal: true,
		// AutoInstrumental fica FALSE: ligar sozinho poria a máquina de quem só
		// enviou músicas para trabalhar horas sem ter pedido.
		AutoInstrumental: false,
	}
}

// Validate enforces the invariants: every fade in [0, 30000]ms and every
// percentage (overlap/talk/normalize) in [0,100].
func (s Settings) Validate() error {
	for _, f := range []int{
		s.MusicaIn, s.MusicaOut, s.VinhetaIn, s.VinhetaOut, s.ComercialIn, s.ComercialOut,
	} {
		if f < 0 {
			return ErrNegativeFade
		}
		if f > maxFadeMs {
			return ErrFadeTooLong
		}
	}
	for _, v := range []int{s.OverlapVol, s.TalkVol, s.NormalizeVol} {
		if v < 0 || v > 100 {
			return ErrVolPercent
		}
	}
	return nil
}
