package stemsep

import (
	"bytes"
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

/*
RunPod separa o vocal numa GPU alugada por segundo.

MEDIDO no spike (RTX 4000 Ada): **~10 s para uma faixa de 3 min** — 0,06× o tempo
real, contra 1,9× num M1 Pro e ~3–5× numa VM da Fly. O acervo de 83 faixas sai por
menos de US$ 0,10, uma vez; a alternativa de VM dedicada custava US$ 124 POR MÊS.

⚠️ O POD É UM SERVIÇO, NÃO UM JOB. Ele sobe uma vez, carrega o modelo e atende
várias faixas; o AutoDJ manda os bytes e recebe os bytes de volta. Isso mantém a
porta `StemSeparator` idêntica (arquivo entra, arquivo sai), dispensa rota nova e
token no AutoDJ, e — o que mais importa no custo — paga os ~3 min de subida UMA vez
por lote, não por faixa.

Como a fila do `StemService` é de um trabalhador só, as chamadas chegam aqui
serializadas: um pod basta.
*/

//go:embed podservice.py
var podService string

// GPUs tentadas em ordem, da mais barata à mais cara (preços consultados na API,
// não na tabela do site — que mostra Serverless e cobra 4× mais).
//
// ⚠️ A LISTA É REQUISITO, NÃO REFINAMENTO: no spike, TRÊS modelos seguidos
// responderam "no longer any instances available", incluindo o primeiro da lista.
// Capacidade de nuvem some e volta; prender o recurso a um modelo o faz falhar por
// motivo que não tem nada a ver com o AutoDJ.
var defaultGPUs = []string{
	"NVIDIA RTX A5000",               // ~US$ 0,16/h
	"NVIDIA RTX A4000",               // ~US$ 0,17/h
	"NVIDIA RTX A4500",               // ~US$ 0,19/h
	"NVIDIA RTX 4000 Ada Generation", // ~US$ 0,20/h — a que o spike pegou
	"NVIDIA GeForce RTX 3090",        // ~US$ 0,22/h
	"NVIDIA GeForce RTX 4090",        // ~US$ 0,34/h
}

const (
	defaultImage = "pytorch/pytorch:2.4.0-cuda12.4-cudnn9-runtime"
	podPort      = 8000
	// Saldo mínimo para lançar. Preflight barato: sem saldo o pod some sozinho e
	// o erro que chega é confuso.
	defaultMinBalance = 1.0
	// Quanto esperar o pod ficar de pé (subir + apt + pip install demucs). Medido
	// em ~3 min; o dobro é margem para rede ruim.
	readyTimeout = 8 * time.Minute
	/* Sem faixa nova por este tempo, o AutoDJ derruba o pod.

	   ⚠️ ESTE NÚMERO É DINHEIRO PARADO, e foi por isso que caiu de 8 para 2 min.
	   O pod fica de pé DE PROPÓSITO entre faixas: subir e instalar o Demucs leva
	   ~3 min pagos, então matá-lo após cada faixa faria um lote de 10 custar 10
	   subidas em vez de uma. Mas o preço do "quente" é GPU ociosa a ~US$ 0,27/h —
	   com 8 min eram ~US$ 0,036 por janela vazia, gasto observado na prática.

	   2 min ainda cobrem o intervalo entre faixas de um lote em sequência (a
	   separação leva ~10 s por faixa) e cortam o desperdício para ~US$ 0,009. Uso
	   esporádico — uma faixa agora, outra daqui a pouco — paga a subida de novo, e
	   isso é o certo: quem processa espaçado não deveria bancar GPU parada. */
	podIdleTimeout = 2 * time.Minute
)

// RunPod implementa outbound.StemSeparator contra um pod GPU do RunPod.
type RunPod struct {
	api        *runpodClient
	gpus       []string
	image      string
	minBalance float64
	http       *http.Client

	mu       sync.Mutex
	podID    string
	endpoint string
	podToken string // segredo compartilhado com ESTE pod; sorteado a cada subida
	lastUse  time.Time     // última faixa enviada — base do desligamento por ociosidade
	stopWD   chan struct{} // fecha o watchdog quando o pod morre
}

/*
NewRunPod devolve o adapter, ou nil quando não há chave configurada.

nil é a degradação graciosa de sempre: sem chave, `Available()` é false, as rotas
nem são registradas e a UI esconde a função — em vez de oferecer um botão que só
pode terminar em erro.
*/
func NewRunPod() *RunPod {
	key := strings.TrimSpace(os.Getenv("AUTODJ_RUNPOD_API_KEY"))
	if key == "" {
		return nil
	}
	gpus := defaultGPUs
	if v := os.Getenv("AUTODJ_RUNPOD_GPUS"); v != "" {
		gpus = splitAndTrim(v)
	}
	img := defaultImage
	if v := os.Getenv("AUTODJ_RUNPOD_IMAGE"); v != "" {
		img = v
	}
	min := defaultMinBalance
	if v := os.Getenv("AUTODJ_RUNPOD_MIN_BALANCE"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			min = f
		}
	}
	return &RunPod{
		api:        newRunpodClient(key),
		gpus:       gpus,
		image:      img,
		minBalance: min,
		// Timeout generoso: a separação em si leva ~10 s, mas a PRIMEIRA requisição
		// espera o modelo carregar. Curto demais aborta justo o começo do lote.
		http: &http.Client{Timeout: 30 * time.Minute},
	}
}

func (r *RunPod) Available() bool { return r != nil && r.api != nil }

// SeparateVocals manda o arquivo ao pod e escreve a resposta em outPath.
func (r *RunPod) SeparateVocals(ctx context.Context, path, outPath string) error {
	if !r.Available() {
		return errors.New("runpod: sem chave configurada")
	}
	endpoint, err := r.ensurePod(ctx)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint+"/separate", bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	r.authorize(req)
	// O pod devolve WAV — mesmo formato do adapter local, porque o caminho de
	// saída (`.instrumental.wav`) é escolhido pelo StemService e vale para os dois.
	req.Header.Set("Accept", "audio/wav")
	r.touch()
	resp, err := r.http.Do(req)
	if err != nil {
		// Pod pode ter morrido (ocioso, ou o RunPod o recolheu). Esquece o
		// endereço para a próxima chamada subir outro em vez de insistir num
		// destino morto.
		r.forget()
		return fmt.Errorf("runpod: falha ao falar com o pod: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("runpod: pod respondeu %d: %s", resp.StatusCode, truncate(string(body), 200))
	}
	if err := os.MkdirAll(dirOf(outPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(outPath, body, 0o644)
}

/*
ensurePod garante um pod de pé e devolve o endereço do serviço.

Reusa o que já está no ar — é o que faz o lote pagar a subida uma vez só.
*/
func (r *RunPod) ensurePod(ctx context.Context) (string, error) {
	r.mu.Lock()
	if r.endpoint != "" {
		ep := r.endpoint
		r.mu.Unlock()
		return ep, nil
	}
	r.mu.Unlock()

	if bal, err := r.api.balance(ctx); err == nil && bal < r.minBalance {
		return "", fmt.Errorf("runpod: saldo US$ %.2f abaixo do mínimo US$ %.2f", bal, r.minBalance)
	}

	/* ⚠️ O POD É PÚBLICO — o RunPod publica a porta em
	   `https://<pod-id>-8000.proxy.runpod.net`, alcançável por qualquer um. Sem
	   autenticação, quem descobrisse o endereço mandaria áudio e usaria a GPU que
	   VOCÊ paga; a única barreira seria o pod id ser difícil de adivinhar, que é
	   segurança por obscuridade.

	   Daí um token aleatório por pod: o serviço recusa (401) o que não o traga. É
	   segredo compartilhado simples, e basta porque o pod vive minutos, o canal é
	   HTTPS e o token nunca é reaproveitado entre pods. */
	tok, err := randomToken()
	if err != nil {
		return "", fmt.Errorf("runpod: sorteio do token do pod: %w", err)
	}

	env := map[string]string{
		"SCRIPT_B64": base64.StdEncoding.EncodeToString([]byte(podService)),
		"POD_TOKEN":  tok,
		/* ⚠️ `RUNPOD_API_KEY` NÃO VAI MAIS PARA O POD (revisão de segurança).
		   Ela é credencial da CONTA INTEIRA — lança pod, destrói pod, lê saldo — e
		   estava sendo entregue a um container alugado com porta pública só para
		   alimentar a auto-terminação, que nós mesmos medimos e sabemos não ser
		   confiável (o `podTerminate` responde `null` mesmo quando funciona, e o
		   RunPod já reiniciou container depois do script terminar).

		   Quem encerra o pod é o watchdog daqui, que confirma a morte por consulta.
		   O relógio do pod continua existindo como rede de proteção para o caso de
		   o AutoDJ morrer antes — só que agora ele apenas SAI do processo, sem
		   chamar a API. Perde-se um backstop de segunda linha; em troca, a chave
		   mais poderosa do sistema deixa de circular. */
		"IDLE_SECONDS": "300",
		"PORT":         strconv.Itoa(podPort),
	}
	// ⚠️ `mkdir -p /workspace` NÃO É DETALHE: com volumeInGb=0 esse diretório não
	// existe, escrever ali falha, o comando morre e o RunPod REINICIA o container
	// em loop. Foi o primeiro erro do spike.
	boot := `bash -lc 'set -uo pipefail; mkdir -p /workspace; ` +
		`echo "$SCRIPT_B64" | base64 -d > /workspace/pod.py; ` +
		`apt-get update -qq >/dev/null 2>&1; apt-get install -y -qq ffmpeg >/dev/null 2>&1; ` +
		`pip install -q --no-cache-dir demucs >/dev/null 2>&1; ` +
		`python /workspace/pod.py'`

	var pod podInfo
	var lastErr error
	// SECURE primeiro (rede e disco melhores); COMMUNITY é o plano B — mais barato
	// e com mais capacidade livre, que é justamente o que falta quando dá
	// SUPPLY_CONSTRAINT.
	for _, cloud := range []string{"SECURE", "COMMUNITY"} {
		for _, gpu := range r.gpus {
			p, err := r.api.launch(ctx, gpu, cloud, r.image, boot, fmt.Sprintf("%d/http", podPort), env)
			if err == nil {
				pod = p
				log.Printf("stem: pod %s no ar (%s, %s, US$ %.3f/h)", p.ID, gpu, cloud, p.CostPerHr)
				break
			}
			lastErr = err
			if !errors.Is(err, ErrNoCapacity) {
				return "", err // erro real (chave inválida, cota) — não adianta tentar outra GPU
			}
		}
		if pod.ID != "" {
			break
		}
	}
	if pod.ID == "" {
		return "", fmt.Errorf("runpod: nenhuma GPU disponível: %w", lastErr)
	}

	// O RunPod expõe a porta por um proxy com endereço previsível — não é preciso
	// descobrir IP nem esperar `runtime.ports` aparecer.
	ep := fmt.Sprintf("https://%s-%d.proxy.runpod.net", pod.ID, podPort)

	r.mu.Lock()
	r.podID, r.endpoint, r.podToken = pod.ID, ep, tok
	/* ⚠️ O RELÓGIO DE OCIOSIDADE COMEÇA AGORA, e esquecer isto derruba o recurso
	   inteiro. `lastUse` só era escrito pelo `touch()` do SeparateVocals, que roda
	   DEPOIS do `waitReady` — ou seja, durante os ~3 min de instalação do Demucs o
	   campo continuava zerado, e `time.Since(zero)` dá ~2562047h. O watchdog lia
	   isso como "ocioso há uma eternidade" e matava o pod recém-nascido.

	   Ficou latente desde o início por sorte de cronômetro: com tique de 2 min, a
	   primeira faixa medida (78 s) chamava `touch()` antes do primeiro tique. Ao
	   baixar o tique para 20 s na 0.2.40, a primeira verificação passou a cair no
	   meio da instalação e o bug virou fatal — nenhum pod sobrevivia. */
	r.lastUse = time.Now()
	r.stopWD = make(chan struct{})
	stop := r.stopWD
	r.mu.Unlock()
	go r.watchdog(pod.ID, stop)

	if err := r.waitReady(ctx, ep); err != nil {
		// Não deixa pod pago de pé por causa de uma subida que não completou.
		r.Shutdown(context.WithoutCancel(ctx))
		return "", err
	}
	return ep, nil
}

// waitReady espera o /health do pod — ou seja, o Demucs instalado e o serviço no ar.
// Sem isso a primeira faixa do lote sairia com erro de conexão.
func (r *RunPod) waitReady(ctx context.Context, endpoint string) error {
	deadline := time.Now().Add(readyTimeout)
	probe := &http.Client{Timeout: 15 * time.Second}
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Second):
		}
		/* ⚠️ ESPERAR O POD FICAR PRONTO **É** ATIVIDADE — e esquecer isto mata o
		   pod pela segunda vez, de outro jeito. A primeira versão do conserto fez
		   o relógio de ociosidade começar no lançamento; só que instalar o Demucs
		   leva ~3 min e o teto são 2, então o watchdog derrubava o pod no meio da
		   instalação (log real: "ocioso há 2m0s", exatamente o teto, com o pod
		   ainda subindo).

		   "Ocioso" tem de significar "ninguém está trabalhando". Enquanto sondamos
		   o /health, alguém está: o pod instala e nós esperamos. Quem limita esta
		   fase é o `readyTimeout` (8 min), não o relógio de ociosidade. */
		r.touch()

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"/health", nil)
		if err != nil {
			return err
		}
		r.authorize(req)
		if resp, err := probe.Do(req); err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
	}
	return fmt.Errorf("runpod: pod não ficou pronto em %s", readyTimeout)
}

/*
watchdog garante que o pod morra. É o achado central do spike.

⚠️ A AUTO-TERMINAÇÃO DO POD NÃO É CONFIÁVEL — medido: o pod terminou o trabalho,
subiu o resultado e o RunPod REINICIOU o container, que refez tudo. E não dá para
detectar de dentro, porque a mutation `podTerminate` responde `null` mesmo quando
funciona.

Logo: quem lançou é quem cobra a morte. Este laço acompanha o pod e, uma vez
pedido o encerramento, insiste até a API dizer que ele não existe mais. A falha
silenciosa aqui é GPU rodando até alguém olhar a fatura.
*/
func (r *RunPod) watchdog(podID string, stop <-chan struct{}) {
	// ⚠️ O TICK TEM DE SER BEM MENOR QUE `podIdleTimeout`, senão a granularidade
	// vira desperdício: com tick e teto iguais, um pod ocioso podia sobreviver até
	// o DOBRO do tempo previsto antes de alguém reparar.
	tick := time.NewTicker(20 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-stop:
			return
		case <-tick.C:
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			p, alive, err := r.api.status(ctx, podID)
			cancel()
			if err != nil {
				continue // rede oscilando não é motivo para matar nem para desistir
			}
			if !alive {
				log.Printf("stem: pod %s não existe mais — encerrando watchdog", podID)
				r.forget()
				return
			}
			_ = p
			/* DESLIGA POR OCIOSIDADE — e quem manda aqui é o AutoDJ.

			   O pod tem o próprio relógio (IDLE_SECONDS), mas o spike mostrou que
			   ele não é confiável: o RunPod reiniciou o container depois de o
			   script terminar. Este lado é a autoridade porque é o único que sabe
			   se ainda há faixa na fila — e porque, se falhar, quem paga a GPU
			   parada é o operador. */
			r.mu.Lock()
			idle := time.Since(r.lastUse)
			r.mu.Unlock()
			if idle > podIdleTimeout {
				log.Printf("stem: pod %s ocioso há %s — desligando", podID, idle.Round(time.Second))
				go r.Shutdown(context.Background())
				return
			}
		}
	}
}

// Shutdown mata o pod e só volta quando a API confirma que ele sumiu.
//
// A confirmação é por CONSULTA, não pela resposta do terminate — que devolve null
// mesmo em sucesso.
func (r *RunPod) Shutdown(ctx context.Context) {
	r.mu.Lock()
	id := r.podID
	stop := r.stopWD
	r.podID, r.endpoint, r.stopWD = "", "", nil
	r.mu.Unlock()
	if id == "" {
		return
	}
	if stop != nil {
		close(stop)
	}
	for i := 0; i < 5; i++ {
		_ = r.api.terminate(ctx, id)
		time.Sleep(3 * time.Second)
		if _, alive, err := r.api.status(ctx, id); err == nil && !alive {
			log.Printf("stem: pod %s terminado", id)
			return
		}
	}
	log.Printf("stem: ⚠️ pod %s pode continuar VIVO — confira em runpod.io (GPU paga)", id)
}

// touch marca uso. O relógio da ociosidade é do AutoDJ porque é ele quem sabe se
// ainda há fila; o pod sozinho só enxerga o silêncio.
func (r *RunPod) touch() {
	r.mu.Lock()
	r.lastUse = time.Now()
	r.mu.Unlock()
}

func (r *RunPod) forget() {
	r.mu.Lock()
	r.podID, r.endpoint = "", ""
	r.mu.Unlock()
}

func splitAndTrim(v string) []string {
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if s := strings.TrimSpace(p); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func dirOf(p string) string {
	if i := strings.LastIndex(p, "/"); i > 0 {
		return p[:i]
	}
	return "."
}

/* authorize carimba o segredo compartilhado do pod ATUAL.

Lê sob o mutex porque o endereço e o token são trocados juntos quando um pod morre
e outro sobe — mandar o token velho para o pod novo daria 401 no meio de um lote. */
func (r *RunPod) authorize(req *http.Request) {
	r.mu.Lock()
	tok := r.podToken
	r.mu.Unlock()
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
}

// randomToken sorteia 32 bytes do gerador criptográfico. `math/rand` não serve:
// é previsível a partir da semente, e este valor é a única barreira entre a GPU
// que o operador paga e a internet aberta.
func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
