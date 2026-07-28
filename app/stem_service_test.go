package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"autodj/adapters/driven/memory"
	"autodj/domain/library"
	"autodj/domain/shared"
)

/* fakeSeparator troca o modelo de verdade (minutos de CPU) por um arquivo
   escrito na hora. `gate` deixa o teste segurar a separação no meio, que é como
   os estados `queued`/`running` ficam observáveis sem depender de timing. */
type fakeSeparator struct {
	available bool
	err       error
	gate      chan struct{}

	mu    sync.Mutex
	calls int
}

func (f *fakeSeparator) Available() bool { return f.available }

func (f *fakeSeparator) SeparateVocals(_ context.Context, _, outPath string) error {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	if f.gate != nil {
		<-f.gate
	}
	if f.err != nil {
		return f.err
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(outPath, []byte("instrumental"), 0o644)
}

func (f *fakeSeparator) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func stemFixture(t *testing.T, sep *fakeSeparator) (*StemService, *memory.TrackRepo) {
	t.Helper()
	repo := memory.New()
	svc := NewStemService(repo, sep, t.TempDir())
	return svc, repo
}

func seedSeparable(t *testing.T, repo *memory.TrackRepo, id string) *library.Track {
	t.Helper()
	tr, err := library.NewTrack(id, shared.SourceLocal, id, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	tr.Path = "/" + id + ".mp3"
	if err := repo.Upsert(context.Background(), tr); err != nil {
		t.Fatal(err)
	}
	return tr
}

func waitStatus(t *testing.T, svc *StemService, id, want string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if st, _ := svc.Status(context.Background(), id); st == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	st, msg := svc.Status(context.Background(), id)
	t.Fatalf("status = %q (%s), esperado %q", st, msg, want)
}

// Caminho feliz: pede, o worker separa, o caminho é gravado na faixa e o
// `Playable` passa a anunciar o instrumental pronto.
func TestStemPrepareGravaOInstrumental(t *testing.T) {
	ctx := context.Background()
	sep := &fakeSeparator{available: true}
	svc, repo := stemFixture(t, sep)
	seedSeparable(t, repo, "m1")
	svc.Start(ctx)
	defer svc.Stop()

	if _, err := svc.Prepare(ctx, "m1"); err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	waitStatus(t, svc, "m1", StemReady)

	got, err := repo.Get(ctx, "m1")
	if err != nil {
		t.Fatal(err)
	}
	if !got.HasInstrumental() {
		t.Fatal("o caminho do instrumental tinha de ficar gravado na faixa")
	}
	if _, err := os.Stat(got.InstrumentalPath); err != nil {
		t.Fatalf("arquivo não foi escrito: %v", err)
	}
}

/*
IDEMPOTÊNCIA — e aqui ela vale minutos de CPU, não microssegundos.

Pedir de novo uma faixa já pronta NÃO pode reprocessar: são ~6 min de máquina
para uma música de 4. E pedir enquanto está rodando não pode enfileirar duas
vezes o mesmo trabalho.
*/
func TestStemPrepareNaoReprocessa(t *testing.T) {
	ctx := context.Background()
	sep := &fakeSeparator{available: true, gate: make(chan struct{})}
	svc, repo := stemFixture(t, sep)
	seedSeparable(t, repo, "m1")
	svc.Start(ctx)

	if _, err := svc.Prepare(ctx, "m1"); err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	waitStatus(t, svc, "m1", StemRunning)

	// Segundo pedido COM o trabalho em curso: devolve o andamento, não duplica.
	st, err := svc.Prepare(ctx, "m1")
	if err != nil {
		t.Fatalf("Prepare 2: %v", err)
	}
	if st != StemRunning {
		t.Fatalf("segundo pedido = %q, esperado %q", st, StemRunning)
	}

	close(sep.gate) // libera a separação
	waitStatus(t, svc, "m1", StemReady)
	svc.Stop()

	// Terceiro pedido, agora PRONTO: responde pronto sem tocar no separador.
	st, err = svc.Prepare(ctx, "m1")
	if err != nil {
		t.Fatalf("Prepare 3: %v", err)
	}
	if st != StemReady {
		t.Fatalf("faixa pronta = %q, esperado %q", st, StemReady)
	}
	if n := sep.count(); n != 1 {
		t.Fatalf("o separador rodou %d vezes; tinha de rodar 1", n)
	}
}

// Falha do modelo vira estado `failed` COM o motivo — e a faixa não fica
// marcada como preparada.
func TestStemFalhaGuardaOMotivo(t *testing.T) {
	ctx := context.Background()
	sep := &fakeSeparator{available: true, err: errors.New("modelo explodiu")}
	svc, repo := stemFixture(t, sep)
	seedSeparable(t, repo, "m1")
	svc.Start(ctx)
	defer svc.Stop()

	if _, err := svc.Prepare(ctx, "m1"); err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	waitStatus(t, svc, "m1", StemFailed)
	_, msg := svc.Status(ctx, "m1")
	if msg == "" {
		t.Fatal("falha tem de guardar o motivo — senão o operador não tem por onde começar")
	}
	got, _ := repo.Get(ctx, "m1")
	if got.HasInstrumental() {
		t.Fatal("falha não pode marcar a faixa como preparada")
	}
}

func TestStemRecusaOQueNaoDaPraSeparar(t *testing.T) {
	ctx := context.Background()

	// Ferramenta ausente: a UI esconde a função, e a rota responde o porquê.
	svc, repo := stemFixture(t, &fakeSeparator{available: false})
	seedSeparable(t, repo, "m1")
	if svc.Available() {
		t.Fatal("sem ferramenta não há disponibilidade")
	}
	if _, err := svc.Prepare(ctx, "m1"); !errors.Is(err, ErrSeparatorUnavailable) {
		t.Fatalf("want ErrSeparatorUnavailable, got %v", err)
	}
	svc.Start(ctx) // sem ferramenta, não sobe worker nenhum
	svc.Stop()

	// Streaming: não há PCM deste lado pra alimentar o modelo.
	svc2, repo2 := stemFixture(t, &fakeSeparator{available: true})
	sp, err := library.NewTrack("sp", shared.SourceSpotify, "x", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	sp.SpotifyURI = "spotify:track:z"
	if err := repo2.Upsert(ctx, sp); err != nil {
		t.Fatal(err)
	}
	if _, err := svc2.Prepare(ctx, "sp"); !errors.Is(err, ErrNotSeparable) {
		t.Fatalf("want ErrNotSeparable, got %v", err)
	}

	// Faixa inexistente propaga o erro do repo.
	if _, err := svc2.Prepare(ctx, "fantasma"); err == nil {
		t.Fatal("faixa inexistente tem de falhar")
	}
}

/*
O ARQUIVO É A VERDADE depois de um restart.

O estado em memória morre com o processo; um instrumental preparado ontem
continua pronto hoje, e o serviço tem de dizer isso sem reprocessar nada.
*/
func TestStemStatusLeDoDiscoAposRestart(t *testing.T) {
	ctx := context.Background()
	sep := &fakeSeparator{available: true}
	svc, repo := stemFixture(t, sep)
	tr := seedSeparable(t, repo, "m1")
	tr.InstrumentalPath = "/já/existia.wav"
	if err := repo.Upsert(ctx, tr); err != nil {
		t.Fatal(err)
	}

	if st, _ := svc.Status(ctx, "m1"); st != StemReady {
		t.Fatalf("status = %q, esperado %q (o arquivo já existe)", st, StemReady)
	}
	// Faixa desconhecida: nunca pedido.
	if st, _ := svc.Status(ctx, "nada"); st != StemIdle {
		t.Fatalf("status = %q, esperado %q", st, StemIdle)
	}
}

// Stop é idempotente e, depois dele, Prepare não aceita mais trabalho — senão o
// pedido ficaria pendurado numa fila que ninguém drena.
func TestStemStopIdempotente(t *testing.T) {
	ctx := context.Background()
	svc, repo := stemFixture(t, &fakeSeparator{available: true})
	seedSeparable(t, repo, "m1")
	svc.Start(ctx)
	svc.Stop()
	svc.Stop()
	if _, err := svc.Prepare(ctx, "m1"); !errors.Is(err, ErrSeparatorUnavailable) {
		t.Fatalf("depois do Stop, Prepare devia recusar; got %v", err)
	}
}

/*
PrepareAll — o que faz o interruptor do ⚙ valer para o acervo QUE JÁ EXISTE.

Sem ele, ligar "processar automaticamente" com 83 músicas na biblioteca não
produziria nada visível (o gatilho automático só pega faixa nova) e o operador
concluiria que a opção não funciona.
*/
func TestPrepareAll(t *testing.T) {
	sep := &fakeSeparator{available: true, gate: make(chan struct{})}
	svc, repo := stemFixture(t, sep)
	ctx := context.Background()
	seedSeparable(t, repo, "a")
	seedSeparable(t, repo, "b")
	// já preparada: não pode voltar pra fila e gastar minutos de CPU de novo.
	done := seedSeparable(t, repo, "c")
	done.InstrumentalPath = filepath.Join(t.TempDir(), "c.wav")
	_ = repo.Upsert(ctx, done)
	// streaming: sem PCM deste lado, não há o que separar.
	sp, err := library.NewTrack("s", shared.SourceSpotify, "DRM", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	sp.SpotifyURI = "spotify:track:x"
	_ = repo.Upsert(ctx, sp)

	svc.Start(ctx)
	defer func() {
		close(sep.gate)
		svc.Stop()
	}()

	if n := svc.PrepareAll(ctx); n != 2 {
		t.Fatalf("esperava 2 enfileiradas (a e b), veio %d", n)
	}
	// idempotente: chamar de novo não duplica o trabalho já em andamento.
	if n := svc.PrepareAll(ctx); n != 0 {
		t.Fatalf("segunda passada não devia enfileirar nada, veio %d", n)
	}
}

// Sem a ferramenta instalada não há o que enfileirar — e isso é o caso de
// produção, onde o recurso nem existe.
func TestPrepareAllIndisponivel(t *testing.T) {
	sep := &fakeSeparator{available: false}
	svc, repo := stemFixture(t, sep)
	seedSeparable(t, repo, "a")
	if n := svc.PrepareAll(context.Background()); n != 0 {
		t.Fatalf("sem separador não enfileira, veio %d", n)
	}
}

/*
Working precisa devolver as PRONTAS, e não só o trabalho vivo.

A FILA de execução é montada do `Playable`, que não pode carregar o estado do
preparo (ele termina minutos depois do enfileiramento) — então é por aqui que ela
descobre quais faixas já têm a versão sem voz. E as prontas saem do REPO, não da
memória: o estado em memória se perde no restart do servidor.
*/
func TestWorkingIncluiProntas(t *testing.T) {
	sep := &fakeSeparator{available: true}
	svc, repo := stemFixture(t, sep)
	ctx := context.Background()
	ready := seedSeparable(t, repo, "pronta")
	ready.InstrumentalPath = filepath.Join(t.TempDir(), "pronta.wav")
	_ = repo.Upsert(ctx, ready)
	seedSeparable(t, repo, "crua")

	w := svc.Working(ctx)
	if w["pronta"] != StemReady {
		t.Fatalf("faixa com arquivo deveria vir como ready: %v", w)
	}
	if _, ok := w["crua"]; ok {
		t.Fatalf("faixa sem preparo não entra no mapa: %v", w)
	}
}
