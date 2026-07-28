package stemsep

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

/*
Este pacote é integration-tier (fora do gate de cobertura): o trabalho dele é falar
com processo externo e com a nuvem. O que dá para testar de verdade sem GPU é o que
JÁ QUEBROU na prática — e é isso que está aqui.
*/

// Sem chave, o adapter nem existe: é o que faz a UI esconder a função em vez de
// oferecer um botão que só pode terminar em "não configurado".
func TestNewRunPodSemChave(t *testing.T) {
	t.Setenv("AUTODJ_RUNPOD_API_KEY", "")
	if r := NewRunPod(); r != nil {
		t.Fatal("sem chave o adapter deve ser nil")
	}
	var nilAdapter *RunPod
	if nilAdapter.Available() {
		t.Fatal("adapter nil não pode se dizer disponível")
	}
}

func TestNewRunPodConfiguravel(t *testing.T) {
	t.Setenv("AUTODJ_RUNPOD_API_KEY", "k")
	t.Setenv("AUTODJ_RUNPOD_GPUS", " NVIDIA A , , NVIDIA B ")
	r := NewRunPod()
	if r == nil || !r.Available() {
		t.Fatal("com chave o adapter existe")
	}
	if len(r.gpus) != 2 || r.gpus[0] != "NVIDIA A" || r.gpus[1] != "NVIDIA B" {
		t.Fatalf("lista de GPUs mal lida: %#v", r.gpus)
	}
}

/*
⚠️ A LISTA DE FALLBACK É REQUISITO, e este teste é o motivo.

No spike, TRÊS modelos seguidos responderam "no longer any instances available" —
capacidade de nuvem some e volta. Um adapter preso a uma GPU falha por motivo que
não tem nada a ver com o AutoDJ.
*/
func TestFallbackDeGPUAteAchar(t *testing.T) {
	var tried []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(body)
		s := string(body)
		switch {
		case strings.Contains(s, "clientBalance"):
			_, _ = w.Write([]byte(`{"data":{"myself":{"clientBalance":50}}}`))
		case strings.Contains(s, "podFindAndDeployOnDemand"):
			switch {
			case strings.Contains(s, "GPU-A"):
				tried = append(tried, "GPU-A")
				_, _ = w.Write([]byte(`{"errors":[{"message":"There are no longer any instances available"}]}`))
			case strings.Contains(s, "GPU-B"):
				tried = append(tried, "GPU-B")
				_, _ = w.Write([]byte(`{"data":{"podFindAndDeployOnDemand":{"id":"pod1","desiredStatus":"RUNNING","costPerHr":0.2}}}`))
			}
		default:
			_, _ = w.Write([]byte(`{"data":{}}`))
		}
	}))
	defer srv.Close()

	c := &runpodClient{apiKey: "k", http: srv.Client()}
	// aponta o cliente para o servidor de teste
	orig := runpodEndpoint
	runpodEndpoint = srv.URL
	defer func() { runpodEndpoint = orig }()

	if _, err := c.launch(context.Background(), "GPU-A", "SECURE", "img", "boot", "8000/http", nil); !errors.Is(err, ErrNoCapacity) {
		t.Fatalf("falta de capacidade tem de virar ErrNoCapacity, veio %v", err)
	}
	p, err := c.launch(context.Background(), "GPU-B", "SECURE", "img", "boot", "8000/http", nil)
	if err != nil || p.ID != "pod1" {
		t.Fatalf("segunda GPU devia subir: %v %#v", err, p)
	}
	if len(tried) != 2 {
		t.Fatalf("esperava duas tentativas, houve %v", tried)
	}
}

// Erro que NÃO é de capacidade (chave inválida, cota) não pode virar "tenta a
// próxima GPU": seria uma bateria de chamadas inúteis escondendo a causa real.
func TestErroRealNaoViraFaltaDeCapacidade(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"errors":[{"message":"Unauthorized"}]}`))
	}))
	defer srv.Close()
	orig := runpodEndpoint
	runpodEndpoint = srv.URL
	defer func() { runpodEndpoint = orig }()

	c := &runpodClient{apiKey: "k", http: srv.Client()}
	_, err := c.launch(context.Background(), "G", "SECURE", "i", "b", "8000/http", nil)
	if err == nil || errors.Is(err, ErrNoCapacity) {
		t.Fatalf("erro de autorização não é falta de capacidade: %v", err)
	}
}

// O corpo da resposta do pod vira o arquivo de saída, e o diretório é criado —
// `StemService` escreve num caminho que pode não existir ainda.
func TestSeparateVocalsEscreveSaida(t *testing.T) {
	pod := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		_, _ = w.Write([]byte("INSTRUMENTAL"))
	}))
	defer pod.Close()

	dir := t.TempDir()
	in := filepath.Join(dir, "in.mp3")
	if err := os.WriteFile(in, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "sub", "out.mp3")

	r := &RunPod{api: &runpodClient{apiKey: "k", http: pod.Client()}, http: pod.Client()}
	r.endpoint = pod.URL // pod já "de pé": este teste é do envio, não do ciclo de vida

	if err := r.SeparateVocals(context.Background(), in, out); err != nil {
		t.Fatalf("SeparateVocals: %v", err)
	}
	got, err := os.ReadFile(out)
	if err != nil || string(got) != "INSTRUMENTAL" {
		t.Fatalf("saída errada: %q %v", got, err)
	}
}

// Pod que responde erro não pode virar arquivo de instrumental corrompido — o
// `StemService` gravaria o caminho e a faixa tocaria lixo.
func TestPodComErroNaoEscreveArquivo(t *testing.T) {
	pod := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "modelo estourou a memória", http.StatusInternalServerError)
	}))
	defer pod.Close()

	dir := t.TempDir()
	in := filepath.Join(dir, "in.mp3")
	_ = os.WriteFile(in, []byte("x"), 0o644)
	out := filepath.Join(dir, "out.mp3")

	r := &RunPod{api: &runpodClient{apiKey: "k", http: pod.Client()}, http: pod.Client()}
	r.endpoint = pod.URL
	if err := r.SeparateVocals(context.Background(), in, out); err == nil {
		t.Fatal("erro do pod tem de propagar")
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatal("não pode existir arquivo de saída quando a separação falhou")
	}
}

/*
⚠️ O POD É PÚBLICO (RunPod publica a porta em proxy.runpod.net), então o token não
é refinamento: é a única coisa entre a GPU que o operador paga e a internet. Este
teste falha se alguém remover o carimbo do cabeçalho.
*/
func TestSeparateVocalsMandaOToken(t *testing.T) {
	var gotAuth string
	pod := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte("INSTRUMENTAL"))
	}))
	defer pod.Close()

	dir := t.TempDir()
	in := filepath.Join(dir, "in.mp3")
	if err := os.WriteFile(in, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := &RunPod{api: &runpodClient{apiKey: "k", http: pod.Client()}, http: pod.Client()}
	r.endpoint = pod.URL
	r.podToken = "segredo-do-pod"

	if err := r.SeparateVocals(context.Background(), in, filepath.Join(dir, "out.wav")); err != nil {
		t.Fatalf("SeparateVocals: %v", err)
	}
	if gotAuth != "Bearer segredo-do-pod" {
		t.Fatalf("token não foi carimbado: %q", gotAuth)
	}
}

// Sem token guardado (pod subido antes desta versão, ou uso manual) o cabeçalho
// não vai — em vez de ir vazio, que o serviço leria como tentativa inválida.
func TestSemTokenNaoMandaCabecalho(t *testing.T) {
	var tinha bool
	pod := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, tinha = r.Header["Authorization"]
		_, _ = w.Write([]byte("X"))
	}))
	defer pod.Close()

	dir := t.TempDir()
	in := filepath.Join(dir, "in.mp3")
	_ = os.WriteFile(in, []byte("x"), 0o644)

	r := &RunPod{api: &runpodClient{apiKey: "k", http: pod.Client()}, http: pod.Client()}
	r.endpoint = pod.URL
	if err := r.SeparateVocals(context.Background(), in, filepath.Join(dir, "o.wav")); err != nil {
		t.Fatal(err)
	}
	if tinha {
		t.Fatal("sem token não pode mandar cabeçalho vazio")
	}
}

// Dois pods nunca compartilham segredo, e o valor tem entropia de sobra.
func TestRandomTokenDistintoEForte(t *testing.T) {
	a, err := randomToken()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := randomToken()
	if a == b {
		t.Fatal("dois sorteios iguais")
	}
	if len(a) != 64 { // 32 bytes em hex
		t.Fatalf("token curto demais: %d chars", len(a))
	}
}

/*
⚠️ REGRESSÃO REAL (v0.2.40 → corrigida na 0.2.44): um pod recém-lançado tinha
`lastUse` zerado até a PRIMEIRA faixa ser enviada, e isso só acontece depois dos
~3 min de instalação do Demucs. `time.Since(tempo zero)` dá ~2562047h, o watchdog
lia "ocioso há uma eternidade" e matava o pod 20 s depois de subir — separação
nenhuma terminava. Este teste falha se alguém tirar a inicialização.
*/
func TestPodNovoNaoNasceOcioso(t *testing.T) {
	r := &RunPod{}
	// estado logo após o lançamento, antes de qualquer faixa
	r.lastUse = time.Now()

	idle := time.Since(r.lastUse)
	if idle > podIdleTimeout {
		t.Fatalf("pod recém-lançado apareceu ocioso há %s", idle)
	}

	// e o inverso: sem inicializar, o valor zero É catastroficamente grande — é o
	// que o watchdog via.
	var zero RunPod
	if time.Since(zero.lastUse) < podIdleTimeout {
		t.Fatal("premissa do teste quebrou: tempo zero deveria parecer ociosidade enorme")
	}
}

/*
⚠️ SEGUNDA FORMA DO MESMO BUG (v0.2.44 → corrigida na 0.2.45). Depois de fazer o
relógio começar no lançamento, o pod passou a morrer EXATAMENTE no teto de
ociosidade (2 min) enquanto ainda instalava o Demucs (~3 min): "ocioso há 2m0s".
Esperar o pod ficar pronto é atividade, não ociosidade — quem limita essa fase é
o readyTimeout, não o relógio de ocioso. Este teste falha se o touch() sair do
laço do waitReady.
*/
func TestEsperarOPodFicarProntoContaComoAtividade(t *testing.T) {
	var probes int
	pod := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		probes++
		if probes < 2 { // primeira sondagem: ainda instalando
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer pod.Close()

	r := &RunPod{api: &runpodClient{apiKey: "k", http: pod.Client()}, http: pod.Client()}
	// relógio "velho": sem o touch() do waitReady, o watchdog já teria matado
	r.lastUse = time.Now().Add(-10 * podIdleTimeout)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := r.waitReady(ctx, pod.URL); err != nil {
		t.Fatalf("waitReady: %v", err)
	}
	if idle := time.Since(r.lastUse); idle > podIdleTimeout {
		t.Fatalf("depois de esperar o pod, ele aparece ocioso há %s — o watchdog o mataria", idle)
	}
}
