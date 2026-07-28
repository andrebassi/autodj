package stemsep

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

/* Este pacote é integration-tier: o trabalho dele é INVOCAR o binário do
   `demucs.cpp`, que o CI não tem. O que dá pra travar sem binário nenhum é a
   lógica de decisão — e é justamente ela que decide se a função APARECE na tela
   e quantos núcleos ela toma da máquina enquanto o playout roda. */

func TestAvailableExigeBinarioEModelo(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "demucs")
	model := filepath.Join(dir, "model.bin")

	cases := []struct {
		name       string
		mkBin      bool
		mkModel    bool
		emptyModel bool
		want       bool
	}{
		{"nada instalado", false, false, false, false},
		// Binário sem modelo é o caso traiçoeiro: a UI ofereceria o botão e a
		// falha só apareceria DEPOIS de o operador esperar o processamento.
		{"binário sem modelo", true, false, false, false},
		{"modelo sem binário", false, true, false, false},
		// Modelo de 0 byte = download interrompido. Vale como ausente.
		{"modelo truncado", true, true, true, false},
		{"os dois inteiros", true, true, false, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			os.Remove(bin)
			os.Remove(model)
			if c.mkBin {
				if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			if c.mkModel {
				body := []byte("pesos")
				if c.emptyModel {
					body = nil
				}
				if err := os.WriteFile(model, body, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			d := &Demucs{Bin: bin, Model: model}
			if got := d.Available(); got != c.want {
				t.Fatalf("Available() = %v, want %v", got, c.want)
			}
		})
	}

	// Diretório no lugar do arquivo não pode passar por instalação válida.
	d := &Demucs{Bin: dir, Model: dir}
	if d.Available() {
		t.Fatal("diretório não é binário nem modelo")
	}
	// Caminho vazio (env não setada e sem home) também não.
	if (&Demucs{}).Available() {
		t.Fatal("sem caminho não há ferramenta")
	}
}

/*
O TETO DE 6 THREADS não é arbitrário: a separação roda AO MESMO TEMPO que o
playout, e tomar a máquina inteira faria a música engasgar. O ganho do upstream
também achata bem antes disso.
*/
func TestThreadsRespeitaOTeto(t *testing.T) {
	if got := (&Demucs{Threads: 32}).threads(); got != 6 {
		t.Fatalf("threads = %d, want 6 (teto)", got)
	}
	if got := (&Demucs{Threads: -1}).threads(); got < 1 || got > 6 {
		t.Fatalf("threads = %d, esperado entre 1 e 6", got)
	}
	if got := (&Demucs{Threads: 2}).threads(); got != 2 {
		t.Fatalf("threads = %d, want 2", got)
	}
	// 0 = "descubra sozinho", limitado ao teto.
	auto := (&Demucs{}).threads()
	want := runtime.NumCPU()
	if want > 6 {
		want = 6
	}
	if auto != want {
		t.Fatalf("threads automático = %d, want %d", auto, want)
	}
}

// Sem ferramenta instalada, SeparateVocals falha CEDO e com mensagem que diz o
// que faltou — não deixa o operador esperando por um processo que nunca roda.
func TestSeparateVocalsSemFerramenta(t *testing.T) {
	err := (&Demucs{}).SeparateVocals(context.Background(), "/a.mp3", "/out.wav")
	if err == nil {
		t.Fatal("sem binário/modelo tem de falhar")
	}
	if !strings.Contains(err.Error(), "indisponível") {
		t.Fatalf("mensagem não explica a causa: %v", err)
	}
}

func TestStemPath(t *testing.T) {
	got := stemPath("/tmp/x", "3_vocals")
	if filepath.Base(got) != "target_3_vocals.wav" {
		t.Fatalf("nome do stem errado: %s", got)
	}
}

// tail existe pra que a falha do ffmpeg/demucs chegue ao operador com o FIM da
// saída (onde o erro fica), e não com "exit status 1" pelado.
func TestTail(t *testing.T) {
	if got := tail("curto", 100); got != "curto" {
		t.Fatalf("texto curto não deve ser cortado: %q", got)
	}
	got := tail(strings.Repeat("a", 50)+"FIM", 3)
	if got != "…FIM" {
		t.Fatalf("tail deveria manter o FIM: %q", got)
	}
}

// New usa os caminhos padrão do instalador, e o env sobrepõe — é o que permite
// apontar para outra instalação sem recompilar.
func TestNewRespeitaEnv(t *testing.T) {
	t.Setenv("AUTODJ_DEMUCS_BIN", "/custom/bin")
	t.Setenv("AUTODJ_DEMUCS_MODEL", "/custom/model")
	d := New()
	if d.Bin != "/custom/bin" || d.Model != "/custom/model" {
		t.Fatalf("env ignorada: %+v", d)
	}
	if d.FFmpeg != "ffmpeg" {
		t.Fatalf("ffmpeg default errado: %q", d.FFmpeg)
	}
}
