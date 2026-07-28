// Package stemsep separa o vocal de um arquivo PCM com o modelo Demucs v4
// (hybrid transformer), rodando o binário `demucs.cpp` fora do processo.
//
// POR QUE SHELL-OUT, e não cgo: `demucs.cpp` é C++17 + Eigen header-only (MIT,
// sem PyTorch), então cgo funcionaria — mas passaria a exigir toolchain C++ para
// compilar o servidor Go, inclusive na imagem de deploy, onde este recurso NEM
// RODA (a VM de produção é `shared-cpu-1x` com 1 GB e a separação leva ~1,4× o
// tempo real). Shell-out mantém o binário do AutoDJ puro e o recurso opt-in.
//
// MEDIDO (M1 Pro, 6 threads): 45s de áudio em 64,8s de parede. É a razão de isto
// nunca ser chamado no caminho do playout — é pré-render, guardado em arquivo.
package stemsep

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
)

// Demucs implements outbound.StemSeparator by shelling out to `demucs.cpp`.
type Demucs struct {
	Bin     string // binário demucs_mt (default ~/.autodj/bin/demucs)
	Model   string // pesos ggml (default ~/.autodj/models/htdemucs.bin)
	FFmpeg  string // usado p/ converter a entrada e somar os stems (default "ffmpeg")
	Threads int    // 0 = núcleos disponíveis (limitado a 6: acima disso não paga)
}

// New returns a Demucs separator with the default install paths (the ones
// `scripts/build-demucs.sh` writes to), overridable by env.
func New() *Demucs {
	home, _ := os.UserHomeDir()
	bin := os.Getenv("AUTODJ_DEMUCS_BIN")
	if bin == "" && home != "" {
		bin = filepath.Join(home, ".autodj", "bin", "demucs")
	}
	model := os.Getenv("AUTODJ_DEMUCS_MODEL")
	if model == "" && home != "" {
		model = filepath.Join(home, ".autodj", "models", "htdemucs.bin")
	}
	/* AUTODJ_DEMUCS_THREADS existe por causa da MEMÓRIA, não da velocidade.

	   Medido: 5,87 GB de pico com 2 threads e 6,98 GB com 4. Numa VM de 8 GB (a de
	   produção depois da v0.2.35) quatro threads deixariam menos de 1 GB para o
	   sistema, o servidor e o SQLite — e um OOM aqui não degrada o recurso, mata a
	   máquina inteira e tira a estação do ar. Onde a folga é curta, o operador
	   fixa um número menor e troca tempo por segurança. */
	threads := 0
	if v := os.Getenv("AUTODJ_DEMUCS_THREADS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			threads = n
		}
	}
	return &Demucs{Bin: bin, Model: model, FFmpeg: "ffmpeg", Threads: threads}
}

// Available reports whether both the binary and the weights are present. As duas
// coisas: binário sem modelo falha na primeira faixa, e o operador só descobriria
// depois de esperar. Melhor a função nem aparecer.
func (d *Demucs) Available() bool {
	if d.Bin == "" || d.Model == "" {
		return false
	}
	if fi, err := os.Stat(d.Bin); err != nil || fi.IsDir() {
		return false
	}
	if fi, err := os.Stat(d.Model); err != nil || fi.IsDir() || fi.Size() == 0 {
		return false
	}
	return true
}

// threads limita a 6 de propósito: o benchmark do upstream mostra o ganho
// achatando bem antes disso, e deixar a máquina inteira ocupada atrapalharia
// justamente o playout, que roda ao mesmo tempo.
func (d *Demucs) threads() int {
	n := d.Threads
	if n <= 0 {
		n = runtime.NumCPU()
	}
	if n > 6 {
		n = 6
	}
	if n < 1 {
		n = 1
	}
	return n
}

/*
SeparateVocals roda o modelo e escreve em outPath o áudio SEM vocal.

Três passos, e cada um existe por um motivo:

 1. converte a entrada p/ WAV estéreo 44,1k — o `demucs.cpp` lê WAV via
    libnyquist, não MP3, e o modelo espera 2 canais a 44,1kHz. **É aqui que a
    faixa MONO passa a funcionar**: ela é duplicada para dois canais e o modelo
    separa por conteúdo, sem depender de imagem estéreo nenhuma.
 2. roda o modelo, que escreve QUATRO stems num diretório
    (`target_{0..3}_{drums,bass,other,vocals}.wav`).
 3. soma drums+bass+other (tudo menos o vocal) num arquivo só.

Somar os três é melhor que subtrair o vocal do original: a subtração reintroduz
o resíduo do que o modelo NÃO conseguiu separar, com fase invertida — o defeito
some do lugar errado e vira artefato audível.
*/
func (d *Demucs) SeparateVocals(ctx context.Context, path, outPath string) error {
	if !d.Available() {
		return fmt.Errorf("stemsep: demucs indisponível (bin=%q model=%q)", d.Bin, d.Model)
	}
	work, err := os.MkdirTemp("", "autodj-stem-")
	if err != nil {
		return fmt.Errorf("stemsep: temp dir: %w", err)
	}
	defer os.RemoveAll(work)

	in := filepath.Join(work, "in.wav")
	if err := d.run(ctx, d.FFmpeg,
		"-y", "-loglevel", "error", "-i", path,
		"-ac", "2", "-ar", "44100", "-c:a", "pcm_s16le", in,
	); err != nil {
		return fmt.Errorf("stemsep: preparar entrada: %w", err)
	}

	outDir := filepath.Join(work, "stems")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("stemsep: out dir: %w", err)
	}
	if err := d.run(ctx, d.Bin, d.Model, in, outDir+string(os.PathSeparator),
		strconv.Itoa(d.threads()),
	); err != nil {
		return fmt.Errorf("stemsep: demucs: %w", err)
	}

	drums, bass, other := stemPath(outDir, "0_drums"), stemPath(outDir, "1_bass"), stemPath(outDir, "2_other")
	for _, p := range []string{drums, bass, other} {
		if _, err := os.Stat(p); err != nil {
			return fmt.Errorf("stemsep: stem ausente %s: %w", filepath.Base(p), err)
		}
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return fmt.Errorf("stemsep: dir de saída: %w", err)
	}
	// `normalize=0` no amix é obrigatório: com a normalização ligada o ffmpeg
	// divide pelo número de entradas e o instrumental sairia ~10 dB abaixo do
	// original — o operador ouviria "sumiu tudo", não "sumiu a voz".
	if err := d.run(ctx, d.FFmpeg,
		"-y", "-loglevel", "error",
		"-i", drums, "-i", bass, "-i", other,
		"-filter_complex", "[0:a][1:a][2:a]amix=inputs=3:duration=longest:normalize=0[out]",
		"-map", "[out]", "-c:a", "pcm_s16le", outPath,
	); err != nil {
		return fmt.Errorf("stemsep: juntar stems: %w", err)
	}
	return nil
}

// stemPath monta o nome que o demucs.cpp escreve (`target_<n>_<nome>.wav`).
func stemPath(dir, suffix string) string {
	return filepath.Join(dir, "target_"+suffix+".wav")
}

// run executa e devolve a saída de erro embutida na mensagem — sem isso, uma
// falha do modelo chega ao operador como "exit status 1" e não dá por onde
// começar.
func (d *Demucs) run(ctx context.Context, bin string, args ...string) error {
	cmd := exec.CommandContext(ctx, bin, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, tail(string(out), 400))
	}
	return nil
}

// tail devolve os últimos n bytes de s (o erro do ffmpeg/demucs vem no fim).
func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "…" + s[len(s)-n:]
}
