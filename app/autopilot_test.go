package app

import (
	"context"
	"sync"
	"testing"
	"time"

	"autodj/adapters/driven/memory"
	"autodj/adapters/driven/stubengine"
	"autodj/domain/library"
	"autodj/domain/playback"
	"autodj/domain/schedule"
	"autodj/domain/shared"
)

// fakeEngine lets tests force the engine health signal (stubengine is always
// healthy). It is a no-op player otherwise.
type fakeEngine struct{ healthy bool }

func (f fakeEngine) Push(context.Context, playback.QueueItem) error { return nil }
func (f fakeEngine) Skip(context.Context) error                    { return nil }
func (f fakeEngine) RemoveAt(context.Context, int) error           { return nil }
func (f fakeEngine) MoveAt(context.Context, int, int) error        { return nil }
func (f fakeEngine) NowPlaying(context.Context) (playback.Playable, error) {
	return playback.Playable{}, nil
}
func (f fakeEngine) Healthy(context.Context) bool { return f.healthy }

// seedLocalMusic seeds a mixable/PCM local music track into repo.
func seedLocalMusic(t *testing.T, repo *memory.TrackRepo, id string) {
	t.Helper()
	tr, err := library.NewTrack(id, shared.SourceLocal, id, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	tr.Path = "/" + id + ".mp3"
	tr.Artist = id
	tr.Category = string(library.CategoryMusic)
	if err := repo.Upsert(context.Background(), tr); err != nil {
		t.Fatal(err)
	}
}

// autopilotFixture builds an Autopilot over memory repos + a real schedule &
// playback service, at instant now, seeding the given local music track ids.
func autopilotFixture(t *testing.T, now time.Time, withGrid bool, ids ...string) (*Autopilot, *ScheduleService, *PlaybackService, *memory.TrackRepo) {
	t.Helper()
	ctx := context.Background()
	tracks := memory.New()
	for _, id := range ids {
		seedLocalMusic(t, tracks, id)
	}
	clocks := memory.NewClockRepo()
	if err := clocks.Save(ctx, okClock("manha")); err != nil {
		t.Fatal(err)
	}
	grids := memory.NewGridRepo()
	sched := NewScheduleService(clocks, grids, tracks, memory.NewEventRepo())
	if withGrid {
		if _, err := sched.SaveGrid(ctx, gridOf(schedule.Cell{Weekday: now.Weekday(), Hour: now.Hour(), ClockID: "manha"})); err != nil {
			t.Fatal(err)
		}
	}
	play := NewPlaybackService(tracks, stubengine.New())
	auto := NewAutopilot(fixedClock{now}, sched, play, tracks, stubengine.New(), 5)
	auto.SetSeed(1)
	return auto, sched, play, tracks
}

func TestAutopilotTickRefillsFromGrid(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 27, 6, 30, 0, 0, time.UTC)
	auto, _, play, _ := autopilotFixture(t, now, true, "m1", "m2", "m3", "m4")

	if err := auto.Tick(ctx); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if len(play.Queue(ctx)) == 0 {
		t.Fatal("refill should fill queue from the grid clock")
	}
	st := auto.Status(ctx)
	if st.LastTick.IsZero() {
		t.Fatal("lastTick should be set after a tick")
	}
	if st.QueueLen == 0 {
		t.Fatal("status queueLen should mirror the filled queue")
	}
}

/*
NEVER MUTE é promessa de ESTAÇÃO: só vale com grade configurada. A grade aqui
aponta para uma hora SEM cell (03h não está na grade salva pelo fixture, que só
cobre `now.Hour()`), então a geração não rende nada e o embaralhamento de
emergência entra para não deixar a programação muda.
*/
func TestAutopilotReplenishNeverMuteComGrade(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 27, 3, 0, 0, 0, time.UTC)
	auto, _, play, _ := autopilotFixture(t, now, true, "m1", "m2", "m3")
	if _, err := auto.SetEnabled(ctx, true); err != nil { // liga (e já dá um tick)
		t.Fatalf("SetEnabled: %v", err)
	}

	if len(play.Queue(ctx)) == 0 {
		t.Fatal("com grade, fila vazia tem de disparar o never-mute")
	}
	if !auto.Enabled() {
		t.Fatal("com grade o piloto NUNCA se desliga sozinho — a estação segue no ar")
	}
}

/*
SEM GRADE a regra é "NUNCA REPETIR O QUE JÁ TOCOU" — nem mais, nem menos.

Este é o bug que o operador reportou como **loop infinito**: o never-mute rodava
sempre que a fila zerava, então o acervo inteiro voltava e recomeçava, para
sempre. Ele não pediu rádio; pediu as músicas dele.
*/
func TestAutopilotSemGradeNaoRepeteOQueJaTocou(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 27, 3, 0, 0, 0, time.UTC)
	auto, _, play, _ := autopilotFixture(t, now, false, "m1", "m2", "m3")

	// 1) Ligar enche a fila com o acervo — é o que o AUTO deve fazer num tocador.
	if _, err := auto.SetEnabled(ctx, true); err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}
	if len(play.Queue(ctx)) != 3 {
		t.Fatalf("ligar o AUTO deveria enfileirar o acervo; veio %d", len(play.Queue(ctx)))
	}

	// 2) O acervo tocou até o fim (fila zerada). AQUI estava o loop.
	if err := play.Clear(ctx); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if err := auto.Tick(ctx); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if n := len(play.Queue(ctx)); n != 0 {
		t.Fatalf("acervo esgotado NÃO pode voltar pra fila (loop infinito); veio %d", n)
	}
	/* ⚠️ E O PILOTO TEM DE CONTINUAR LIGADO. Uma versão anterior deste fix
	   desligava aqui, e isso quebrou o IMPORT: `enqueueSafe` só põe a estação no ar
	   ao enfileirar quando o AUTO está ligado, então a música importada entrava na
	   fila e ficava parada ("importo e não toca mais"). */
	if !auto.Enabled() {
		t.Fatal("o piloto NÃO pode se desligar sozinho — desligado, o import para de tocar")
	}

	/* 3) LIGAR QUEM JÁ ESTÁ LIGADO não é pedido de tocar tudo de novo. O endpoint
	   é idempotente e o cliente o chama no boot de cada carga de página — se isso
	   re-armasse o despejo, o loop voltaria por uma porta lateral (foi exatamente
	   o que o teste contra o servidor pegou). */
	if _, err := auto.SetEnabled(ctx, true); err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}
	if n := len(play.Queue(ctx)); n != 0 {
		t.Fatalf("ligar o que já está ligado não pode re-despejar o acervo; veio %d", n)
	}

	// 4) DESLIGAR e LIGAR de novo — aí sim é o pedido explícito, e tem de encher.
	if _, err := auto.SetEnabled(ctx, false); err != nil {
		t.Fatalf("SetEnabled(false): %v", err)
	}
	if _, err := auto.SetEnabled(ctx, true); err != nil {
		t.Fatalf("SetEnabled(true): %v", err)
	}
	if len(play.Queue(ctx)) != 3 {
		t.Fatalf("religar o AUTO deveria encher de novo; veio %d", len(play.Queue(ctx)))
	}
}

/*
FAIXA NOVA TEM DE TOCAR — o caso que eu quebrei duas vezes.

Cenário exato do operador: acervo tocou até o fim, ele clica em **Limpar** e
adiciona 3 músicas. Elas TÊM de entrar na fila. Uma trava booleana de "já enchi
uma vez" bloqueava isso, e o app ficava mudo com a biblioteca cheia.

O segundo passo é a parte cruel: os ids são `sha1(path)`, então **reenviar o
MESMO arquivo devolve o MESMO id**. Sem podar o conjunto contra o catálogo, a
faixa reenviada passaria por já-tocada e nunca entraria.
*/
func TestAutopilotFaixaNovaDepoisDoAcervoEsgotadoEntra(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 27, 3, 0, 0, 0, time.UTC)
	auto, _, play, tracks := autopilotFixture(t, now, false, "m1", "m2")

	if _, err := auto.SetEnabled(ctx, true); err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}
	if err := play.Clear(ctx); err != nil { // tocou tudo
		t.Fatalf("Clear: %v", err)
	}

	// 1) música NOVA adicionada depois → entra sozinha no próximo tick.
	seedLocalMusic(t, tracks, "m3")

	if err := auto.Tick(ctx); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	q := play.Queue(ctx)
	if len(q) != 1 || q[0].Playable.TrackID != "m3" {
		t.Fatalf("faixa nova tinha de entrar sozinha; fila=%+v", q)
	}

	// 2) "Limpar" + reenviar o MESMO arquivo (mesmo id) também tem de tocar.
	if err := play.Clear(ctx); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if err := tracks.DeleteAll(ctx); err != nil {
		t.Fatalf("DeleteAll: %v", err)
	}
	seedLocalMusic(t, tracks, "m1") // id repetido, arquivo reenviado
	if err := auto.Tick(ctx); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	q = play.Queue(ctx)
	if len(q) != 1 || q[0].Playable.TrackID != "m1" {
		t.Fatalf("faixa reenviada após Limpar tinha de entrar; fila=%+v", q)
	}
	// E a origem que o operador lê não é "Emergência" — é o acervo dele.
	if got := q[0].Playable.Origin; got != "Acervo" {
		t.Fatalf("origem = %q, esperado \"Acervo\"", got)
	}
}

/*
ADICIONAR COM A FILA CHEIA também tem de entrar — o bug que o operador viu como
"a fila parou de funcionar".

Eu só repunha a fila quando ela estava VAZIA. Junto com o filtro de "já tocou",
isso produzia o pior sintoma possível: das 3 músicas adicionadas, a primeira
entrava, a fila deixava de estar vazia, e as outras duas ficavam presas até ela
acabar. Do lado de fora parece trava, não regra.
*/
func TestAutopilotAcrescentaMesmoComFilaNaoVazia(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 27, 3, 0, 0, 0, time.UTC)
	auto, _, play, tracks := autopilotFixture(t, now, false, "m1")

	if _, err := auto.SetEnabled(ctx, true); err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}
	if len(play.Queue(ctx)) != 1 {
		t.Fatalf("acervo inicial deveria entrar; veio %d", len(play.Queue(ctx)))
	}

	// Fila NÃO está vazia — e mesmo assim as novas têm de ser acrescentadas.
	seedLocalMusic(t, tracks, "m2")
	seedLocalMusic(t, tracks, "m3")
	if err := auto.Tick(ctx); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if n := len(play.Queue(ctx)); n != 3 {
		t.Fatalf("as 3 deveriam estar na fila; veio %d", n)
	}

	// E o tick seguinte não duplica nada — acrescentar sempre ≠ repetir.
	if err := auto.Tick(ctx); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if n := len(play.Queue(ctx)); n != 3 {
		t.Fatalf("tick repetido não pode duplicar; veio %d", n)
	}
}

/*
CatalogChanged é o atalho que tira a espera de 15s: faixa nova entra na fila no
instante do upload, não no próximo tick.

Roda em goroutine (não pode pendurar a resposta do upload), então o teste espera o
efeito em vez de assumir sincronia. E com o piloto DESLIGADO não faz nada — quem
não quer piloto não quer nem o atalho.
*/
func TestAutopilotCatalogChangedEnfileiraNaHora(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 27, 3, 0, 0, 0, time.UTC)
	auto, _, play, tracks := autopilotFixture(t, now, false)

	// Desligado: o aviso é ignorado.
	seedLocalMusic(t, tracks, "m1")
	auto.CatalogChanged()
	time.Sleep(30 * time.Millisecond)
	if n := len(play.Queue(ctx)); n != 0 {
		t.Fatalf("piloto desligado não pode enfileirar; veio %d", n)
	}

	// Ligado: o aviso põe a faixa nova na fila sem esperar o tick.
	if _, err := auto.SetEnabled(ctx, true); err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}
	seedLocalMusic(t, tracks, "m2")
	auto.CatalogChanged()
	deadline := time.Now().Add(2 * time.Second)
	for len(play.Queue(ctx)) < 2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if n := len(play.Queue(ctx)); n != 2 {
		t.Fatalf("faixa nova deveria entrar na hora; fila=%d", n)
	}
}

/*
AVISOS CONCORRENTES NÃO PODEM DUPLICAR — um envio em lote manda um por arquivo.

Bug medido no app: 3 arquivos enviados juntos produziram uma fila de 5. Cada
upload disparava `CatalogChanged`, cada um com seu Tick em goroutine, e dois
ticks liam a mesma faixa como nova antes de qualquer um marcar (check-then-act).

⚠️ O `-race` NÃO pega isso: os acessos são protegidos por mutex; a corrida é de
LÓGICA. Por isso o teste conta itens em vez de confiar no detector.
*/
func TestAutopilotAvisosConcorrentesNaoDuplicam(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 27, 3, 0, 0, 0, time.UTC)
	auto, _, play, tracks := autopilotFixture(t, now, false, "m1", "m2", "m3")
	if _, err := auto.SetEnabled(ctx, true); err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}
	if err := play.Clear(ctx); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	// zera a memória como um "Limpar" faria, pra as 3 valerem como novas
	if _, err := auto.SetEnabled(ctx, false); err != nil {
		t.Fatal(err)
	}
	auto.mu.Lock()
	auto.enabled = true // liga sem passar pelo Tick imediato
	auto.dumped = nil
	auto.mu.Unlock()

	seedLocalMusic(t, tracks, "m4") // uma quarta, pra o lote não ser trivial

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ { // 8 avisos simultâneos, como um lote de 8 arquivos
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := auto.Tick(ctx); err != nil {
				t.Errorf("Tick: %v", err)
			}
		}()
	}
	wg.Wait()

	q := play.Queue(ctx)
	if len(q) != 4 {
		t.Fatalf("4 faixas no acervo têm de virar 4 itens na fila; veio %d", len(q))
	}
	seen := map[string]bool{}
	for _, it := range q {
		if seen[it.Playable.TrackID] {
			t.Fatalf("faixa duplicada na fila: %s", it.Playable.TrackID)
		}
		seen[it.Playable.TrackID] = true
	}
}

func TestAutopilotTickEmptyCatalogStaysSilent(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 27, 3, 0, 0, 0, time.UTC)
	auto, _, play, _ := autopilotFixture(t, now, false) // no tracks, no grid

	if err := auto.Tick(ctx); err != nil {
		t.Fatalf("Tick with empty catalog must not error: %v", err)
	}
	if len(play.Queue(ctx)) != 0 {
		t.Fatal("empty catalog → queue stays empty (silent no-op)")
	}
}

func TestAutopilotTickInjectsDueEventInFront(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 27, 6, 30, 0, 0, time.UTC)
	auto, sched, play, _ := autopilotFixture(t, now, true, "m1", "m2", "m3", "m4")

	ev := schedule.ScheduledEvent{
		ID:       "e1",
		Kind:     schedule.EventComercial,
		Days:     []time.Weekday{now.Weekday()},
		Hour:     now.Hour(),
		Minute:   now.Minute(),
		Priority: 90,
		MediaRef: "m1",
	}
	if _, err := sched.SaveEvent(ctx, ev); err != nil {
		t.Fatal(err)
	}

	if err := auto.Tick(ctx); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	q := play.Queue(ctx)
	if len(q) == 0 || q[0].Playable.TrackID != "m1" {
		t.Fatalf("due event should be injected in front, queue=%+v", q)
	}
}

func TestAutopilotTickUnhealthyEngineKeepsSupervising(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 27, 3, 0, 0, 0, time.UTC)
	tracks := memory.New()
	seedLocalMusic(t, tracks, "m1")
	clocks := memory.NewClockRepo()
	_ = clocks.Save(ctx, okClock("manha"))
	grids := memory.NewGridRepo()
	sched := NewScheduleService(clocks, grids, tracks, memory.NewEventRepo())
	// COM grade: é o cenário de estação, o único em que o never-mute atua e em que
	// "seguir supervisionando com o motor doente" quer dizer alguma coisa.
	if _, err := sched.SaveGrid(ctx, gridOf(schedule.Cell{Weekday: now.Weekday(), Hour: now.Hour(), ClockID: "manha"})); err != nil {
		t.Fatal(err)
	}
	play := NewPlaybackService(tracks, fakeEngine{healthy: false})
	auto := NewAutopilot(fixedClock{now}, sched, play, tracks, fakeEngine{healthy: false}, 5)
	auto.SetSeed(1)

	if err := auto.Tick(ctx); err != nil {
		t.Fatalf("unhealthy engine must not fail the tick: %v", err)
	}
	if len(play.Queue(ctx)) == 0 {
		t.Fatal("supervision continues despite unhealthy engine")
	}
}

func TestAutopilotSetEnabledTicksImmediately(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 27, 6, 30, 0, 0, time.UTC)
	auto, _, play, _ := autopilotFixture(t, now, true, "m1", "m2", "m3")

	st, err := auto.SetEnabled(ctx, true)
	if err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}
	if !st.Enabled {
		t.Fatal("status should report enabled")
	}
	if !auto.Enabled() {
		t.Fatal("Enabled() should be true")
	}
	if len(play.Queue(ctx)) == 0 {
		t.Fatal("enabling should tick immediately and fill the queue")
	}
	if st.LastTick.IsZero() {
		t.Fatal("immediate tick should set lastTick")
	}

	st2, err := auto.SetEnabled(ctx, false)
	if err != nil {
		t.Fatalf("disable: %v", err)
	}
	if st2.Enabled || auto.Enabled() {
		t.Fatal("status should report disabled")
	}
}

func TestNewAutopilotDefaultLowWater(t *testing.T) {
	now := time.Date(2026, 7, 27, 3, 0, 0, 0, time.UTC)
	tracks := memory.New()
	sched := NewScheduleService(memory.NewClockRepo(), memory.NewGridRepo(), tracks, memory.NewEventRepo())
	play := NewPlaybackService(tracks, stubengine.New())
	auto := NewAutopilot(fixedClock{now}, sched, play, tracks, stubengine.New(), 0)
	if auto.lowWater != defaultLowWater {
		t.Fatalf("lowWater = %d, want default %d", auto.lowWater, defaultLowWater)
	}
}

func TestAutopilotRunCancels(t *testing.T) {
	now := time.Date(2026, 7, 27, 3, 0, 0, 0, time.UTC)
	auto, _, _, _ := autopilotFixture(t, now, false, "m1")
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled → Run returns on the first select
	done := make(chan struct{})
	go func() { auto.Run(ctx, 0); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run should return promptly when ctx is cancelled")
	}
}

func TestAutopilotRunTicksWhileEnabled(t *testing.T) {
	now := time.Date(2026, 7, 27, 6, 30, 0, 0, time.UTC)
	auto, _, play, _ := autopilotFixture(t, now, true, "m1", "m2", "m3")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	auto.mu.Lock()
	auto.enabled = true
	auto.mu.Unlock()

	go auto.Run(ctx, time.Millisecond)

	// poll (bounded) until the ticker has run at least once and filled the queue.
	deadline := time.Now().Add(2 * time.Second)
	for {
		if len(play.Queue(ctx)) > 0 && !auto.Status(ctx).LastTick.IsZero() {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("Run should tick and fill the queue while enabled")
		}
		time.Sleep(5 * time.Millisecond)
	}
}
