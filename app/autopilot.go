package app

import (
	"context"
	"log"
	"math/rand"
	"sync"
	"time"

	"autodj/domain/library"
	"autodj/domain/schedule"
	"autodj/ports/inbound"
	"autodj/ports/outbound"
)

// defaultLowWater is the queue depth below which the autopilot refills from the
// current grid hour. Chosen so a crossfaded queue never runs dry between ticks.
const defaultLowWater = 5

// defaultInterval is the supervision cadence when Run is called with interval<=0.
const defaultInterval = 15 * time.Second

// Autopilot implements inbound.AutopilotService: unattended supervision built on
// top of the schedule + playback services. Each Tick runs the pure
// schedule.Decide policy against the live queue/grid/events and executes it —
// injecting due events in front (priority order), refilling from the current
// grid hour when low, and replenishing from the catalog rather than letting the
// queue drain to silence — com grade isso é o NEVER MUTE do Épico 4 (repete
// quantas vezes precisar); sem grade, só entra o que ainda não tocou.
// Engine/content failures degrade (logged) instead of stopping supervision.
type Autopilot struct {
	clock    outbound.Clock
	sched    *ScheduleService
	play     *PlaybackService
	catalog  outbound.TrackRepo
	engine   outbound.PlaybackEngine
	lowWater int
	seed     int64 // replenish-shuffle seed; 0 derives from the tick clock

	// tickMu serializa o Tick (ver o comentário lá). Separado do `mu` de propósito:
	// um Tick longo não pode segurar quem só quer ler o Status.
	tickMu sync.Mutex

	mu       sync.Mutex
	enabled  bool
	lastTick time.Time
	/* dumped são os ids que o piloto JÁ despejou na fila nesta sessão, sem grade.

	   A regra que este conjunto codifica é "NUNCA REPETIR O QUE JÁ TOCOU" — e não
	   "encher uma vez só", que foi a minha primeira tentativa e quebrou o caso
	   óbvio: o operador limpa a estação, adiciona 3 músicas, e elas não entram na
	   fila porque uma trava booleana de antes ainda estava de pé. Faixa nova é
	   nova, e tem de tocar.

	   É DESCARTADO INTEIRO quando o catálogo muda debaixo dele (ver `syncDumped`)
	   e na transição desligado→ligado, que é o pedido explícito de tocar tudo
	   outra vez. */
	dumped map[string]bool
}

// NewAutopilot wires the autopilot over the schedule + playback services, the
// catalog repo (source of the replenish pool), the engine (health probe)
// and a clock. lowWater<=0 uses the default. It starts disabled — SetEnabled or
// the AUTODJ_AUTOPILOT wiring flag starts it.
func NewAutopilot(clock outbound.Clock, sched *ScheduleService, play *PlaybackService, catalog outbound.TrackRepo, engine outbound.PlaybackEngine, lowWater int) *Autopilot {
	if lowWater <= 0 {
		lowWater = defaultLowWater
	}
	return &Autopilot{clock: clock, sched: sched, play: play, catalog: catalog, engine: engine, lowWater: lowWater}
}

// SetSeed fixes the replenish-shuffle seed for deterministic tests. Production
// leaves it 0 (derived from the tick time).
func (a *Autopilot) SetSeed(seed int64) { a.seed = seed }

// Status reports the current enabled flag, live queue depth and last tick time.
func (a *Autopilot) Status(ctx context.Context) inbound.AutopilotStatus {
	a.mu.Lock()
	defer a.mu.Unlock()
	return inbound.AutopilotStatus{
		Enabled:  a.enabled,
		QueueLen: len(a.play.Queue(ctx)),
		LastTick: a.lastTick,
	}
}

/*
syncDumped zera a memória de "já tocou" quando o CATÁLOGO MUDOU debaixo dela.

O gatilho é qualquer id despejado que não esteja mais no catálogo — sinal de que
o operador apagou faixa ou clicou em **Limpar**. Nesse caso a memória inteira é
descartada, não só a entrada que sumiu.

Descartar TUDO, e não podar item a item, é deliberado: os ids são `sha1(path)`,
então **reenviar o mesmo arquivo devolve o mesmo id**. Uma poda seletiva deixaria
esse id na memória (ele está "vivo" de novo) e a faixa reenviada nunca entraria na
fila — foi exatamente o que o operador viu depois de limpar e adicionar 3 músicas.

O custo do exagero é conhecido e aceito: apagar UMA faixa re-arma um despejo do
acervo inteiro na próxima vez que a fila zerar. Entre repetir música e ficar mudo
com a biblioteca cheia, repetir é o erro barato.
*/
func (a *Autopilot) syncDumped(alive map[string]bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for id := range a.dumped {
		if !alive[id] {
			a.dumped = nil
			return
		}
	}
}

/*
claimUndumped MARCA E DEVOLVE, numa única seção crítica, os ids que ainda não
tinham sido despejados.

Tem de ser atômico. Consultar "já entrou?" e marcar depois é um clássico
check-then-act, e aqui ele acontece de verdade: um envio em LOTE dispara um
`CatalogChanged` por arquivo, cada um com seu Tick em goroutine. Dois ticks liam a
mesma faixa como nova antes de qualquer um marcar, e ela entrava DUAS vezes —
medido no envio de 3 arquivos, que produziu uma fila de 5.

⚠️ O `-race` do Go NÃO pega isto: os dois acessos são protegidos por mutex. A
corrida é de LÓGICA, não de memória.
*/
func (a *Autopilot) claimUndumped(ids []string) []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.dumped == nil {
		a.dumped = make(map[string]bool, len(ids))
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if a.dumped[id] {
			continue
		}
		a.dumped[id] = true
		out = append(out, id)
	}
	return out
}

// releaseDumped devolve ao pool os ids que falharam ao entrar na fila — senão uma
// falha transitória de enfileiramento aposentaria a faixa para o resto da sessão.
func (a *Autopilot) releaseDumped(ids []string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, id := range ids {
		delete(a.dumped, id)
	}
}

/*
CatalogChanged é o aviso de que o acervo ganhou faixa (upload/scan/import), vindo
do LibraryService. Roda um Tick NA HORA.

Sem isto, adicionar música com a fila vazia deixava a estação muda por até 15
segundos — o intervalo de supervisão. Do lado do operador isso é indistinguível de
travamento: ele adiciona e não acontece nada.

EM GOROUTINE, de propósito: o Tick lê repositório e enfileira, e isso não pode
pendurar a resposta do UPLOAD. Um envio em lote dispara um aviso por arquivo; o
custo é um tick a mais por faixa, e o Tick é idempotente — o filtro de "já entrou"
garante que nada duplica.

Desligado, não faz nada: quem não quer piloto não quer nem este atalho.
*/
func (a *Autopilot) CatalogChanged() {
	if !a.Enabled() {
		return
	}
	go func() {
		if err := a.Tick(context.Background()); err != nil {
			log.Printf("autopilot: tick pós-acervo: %v", err)
		}
	}()
}

// Enabled reports whether the loop is currently running ticks.
func (a *Autopilot) Enabled() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.enabled
}

// SetEnabled toggles supervision. Turning it on runs one Tick immediately so the
// queue starts filling without waiting for the loop interval; a tick error is
// returned but the enabled flag stays set (supervision keeps running).
func (a *Autopilot) SetEnabled(ctx context.Context, enabled bool) (inbound.AutopilotStatus, error) {
	a.mu.Lock()
	was := a.enabled
	a.enabled = enabled
	/* Só a TRANSIÇÃO desligado→ligado zera a marca do acervo. Ligar é o pedido
	   explícito de tocar tudo (inclusive de novo, depois de ter acabado) — mas
	   "garantir que está ligado" NÃO é esse pedido.

	   A diferença não é teórica: o cliente chama `setAutopilot(true)` no boot, e
	   este endpoint é idempotente por natureza. Zerando a marca em toda chamada,
	   cada carga de página re-armava um despejo inteiro do acervo — foi assim que
	   o loop reapareceu no teste contra o servidor, mesmo com o guarda no lugar. */
	if enabled && !was {
		a.dumped = nil
	}
	a.mu.Unlock()
	if enabled {
		if err := a.Tick(ctx); err != nil {
			return a.Status(ctx), err
		}
	}
	return a.Status(ctx), nil
}

// Tick runs one supervision cycle: inject due events in front, refill from the
// grid when the queue is low, and guarantee the queue is never left empty while
// catalog exists. Idempotent and safe to call in a loop. A sick engine (not
// Healthy) is logged and supervision continues; content/engine errors degrade to
// best-effort rather than failing the tick.
func (a *Autopilot) Tick(ctx context.Context) error {
	/* UM TICK POR VEZ. O laço de supervisão, o `SetEnabled` e cada aviso de acervo
	   novo podem dispará-lo ao mesmo tempo — um envio em LOTE manda um aviso por
	   arquivo. Ticks concorrentes leem a fila, decidem e enfileiram em paralelo, e
	   o resultado é conteúdo duplicado (medido: 3 arquivos viraram 5 itens).

	   Este mutex é SÓ do Tick, separado do `a.mu` que protege os campos: um Tick
	   longo não pode bloquear quem só quer ler o Status. */
	a.tickMu.Lock()
	defer a.tickMu.Unlock()

	now := a.clock.Now()

	if a.engine != nil && !a.engine.Healthy(ctx) {
		log.Printf("autopilot: engine unhealthy at %s — supervising anyway", now.Format(time.RFC3339))
	}

	// Events due now, resolved to injectable local tracks (priority order).
	injections, err := a.sched.NextInjections(ctx, now)
	if err != nil {
		log.Printf("autopilot: injections: %v", err) // degrade; keep supervising
		injections = nil
	}

	// Live queue depth + grid presence for the current hour feed the policy.
	queueLen := len(a.play.Queue(ctx))
	grid, _ := a.sched.Grid(ctx)
	clockID, hasGrade := grid.ClockAt(now.Weekday(), now.Hour())
	due := a.dueEvents(ctx, now)

	decision := schedule.Decide(now, queueLen, a.lowWater, due, hasGrade)

	// Inject in front, preserving priority order (best-effort per item).
	if len(decision.Inject) > 0 {
		for i := range injections {
			if err := a.play.Enqueue(ctx, injections[i].ID, i, false, "Evento"); err != nil {
				log.Printf("autopilot: inject %s: %v", injections[i].ID, err)
			}
		}
	}

	// Refill the tail from the current grid hour when the queue is low.
	if decision.Refill {
		a.refill(ctx, clockID)
	}

	/* Reposição da fila — e os dois modos diferem no GATILHO, não só no filtro.

	   • ESTAÇÃO (com grade): vale o NEVER MUTE do Épico 4. Só entra quando a fila
	     ZEROU, e aí repete o acervo quantas vezes precisar — silêncio no ar é o
	     defeito que essa regra existe pra evitar.

	   • TOCADOR (sem grade): acrescenta o que AINDA NÃO ENTROU, **tenha ou não fila
	     no momento**. Esperar a fila esvaziar era um bug de verdade: o operador
	     adicionava 3 músicas, a primeira entrava, a fila deixava de estar vazia e as
	     outras duas ficavam presas até ela acabar. Ele via "adicionei 3, entrou 1" e
	     concluiu, com razão, que a fila tinha parado de funcionar.

	     Como o filtro só deixa passar o que nunca entrou, acrescentar sempre NÃO
	     traz o loop de volta: acervo esgotado ⇒ nada novo ⇒ nada acontece.

	   O piloto fica LIGADO nos dois casos. Desligá-lo aqui foi tentado e quebrou o
	   import: `enqueueSafe` (page.tsx) só põe a estação no ar ao enfileirar quando
	   o AUTO está ligado, então com o piloto desligado por baixo a música entrava
	   na fila e ficava parada ("importo e não toca mais"). */
	if hasGrade {
		if len(a.play.Queue(ctx)) == 0 {
			a.replenish(ctx, now, false)
		}
	} else {
		a.replenish(ctx, now, true)
	}

	a.mu.Lock()
	a.lastTick = now
	a.mu.Unlock()
	return nil
}

// dueEvents lists the events due at now (best-effort; a repo error yields none —
// the caller still refills/emergencies as needed).
func (a *Autopilot) dueEvents(ctx context.Context, now time.Time) []schedule.ScheduledEvent {
	events, err := a.sched.Events(ctx)
	if err != nil {
		log.Printf("autopilot: list events: %v", err)
		return nil
	}
	return schedule.DueEvents(events, now)
}

// refill generates the current grid hour's clock into tracks and appends the
// resulting (already mixable/PCM-filtered) tracks to the tail of the queue. A
// generation error or empty result adds nothing — the never-mute guard covers
// the drained case via replenish.
func (a *Autopilot) refill(ctx context.Context, clockID string) {
	tracks, err := a.sched.GeneratePlaylist(ctx, clockID)
	if err != nil {
		log.Printf("autopilot: generate %s: %v", clockID, err)
		return
	}
	for i := range tracks {
		if err := a.play.Enqueue(ctx, tracks[i].ID, -1, false, "Grade"); err != nil {
			log.Printf("autopilot: refill %s: %v", tracks[i].ID, err)
		}
	}
}

// replenish refills the queue from the catalog: it loads every mixable/PCM track in the
// catalog, shuffled deterministically, so the station keeps playing even when the
// grid generation fails. A truly empty (or all-DRM) catalog leaves the queue
// empty — a silent no-op, since there is genuinely nothing to play.
//
// `onlyNew` liga o filtro do tocador: só entram faixas que este piloto ainda NÃO
// despejou nesta sessão. Com ele desligado (estação com grade), o acervo repete —
// que é o que never-mute quer dizer.
//
// Devolve quantas faixas entraram. 0 com `onlyNew` significa "nada novo por
// enquanto" — não é erro, e o piloto segue ligado esperando o que chegar.
func (a *Autopilot) replenish(ctx context.Context, now time.Time, onlyNew bool) int {
	all, err := a.catalog.List(ctx)
	if err != nil {
		log.Printf("autopilot: replenish list: %v", err)
		return 0
	}
	// Antes de filtrar: se algo sumiu do catálogo (apagou faixa, clicou em Limpar),
	// a memória de "já tocou" está velha e é descartada inteira — senão o mesmo
	// arquivo reenviado (mesmo sha1, mesmo id) nunca voltaria a tocar.
	alive := make(map[string]bool, len(all))
	for _, t := range all {
		if t != nil {
			alive[t.ID] = true
		}
	}
	a.syncDumped(alive)

	byID := make(map[string]*library.Track, len(all))
	ids := make([]string, 0, len(all))
	for _, t := range all {
		if t == nil || !t.Mixable || !t.PCMAvailable {
			continue
		}
		byID[t.ID] = t
		ids = append(ids, t.ID)
	}
	// RESERVA os ids numa só operação atômica (ver claimUndumped): é o que impede
	// dois ticks concorrentes de enfileirarem a MESMA faixa duas vezes.
	if onlyNew {
		ids = a.claimUndumped(ids)
	}
	pool := make([]*library.Track, 0, len(ids))
	for _, id := range ids {
		pool = append(pool, byID[id])
	}
	if len(pool) == 0 {
		return 0 // nothing playable; stay silent rather than invent content
	}
	seed := a.seed
	if seed == 0 {
		seed = now.UnixNano()
	}
	rng := rand.New(rand.NewSource(seed))
	rng.Shuffle(len(pool), func(i, j int) { pool[i], pool[j] = pool[j], pool[i] })
	/* Rótulo de ORIGEM que o operador lê na coluna da fila, ao lado de "Grade" e
	   "Evento".

	   "Emergência" saiu de cena. Era o nome da REGRA (never-mute do Épico 4), não
	   do conteúdo, e herdado da identidade de rádio — a mesma que produziu o loop
	   infinito. Ver a própria biblioteca entrar marcada como "Emergência" alarma
	   sem informar nada.

	   • sem grade (tocador): **Acervo** — é literalmente de onde veio.
	   • com grade (estação): **Reserva** — vocabulário de rádio de verdade para a
	     lista que segura o ar, e que diz o papel sem gritar. */
	origin := "Reserva"
	if onlyNew {
		origin = "Acervo"
	}
	n, failed := 0, make([]string, 0)
	for _, t := range pool {
		if err := a.play.Enqueue(ctx, t.ID, -1, false, origin); err != nil {
			log.Printf("autopilot: replenish %s: %v", t.ID, err)
			failed = append(failed, t.ID)
			continue
		}
		n++
	}
	// A reserva já aconteceu lá em cima; aqui só devolvemos o que não entrou, pra
	// uma falha passageira não aposentar a faixa pelo resto da sessão.
	if onlyNew && len(failed) > 0 {
		a.releaseDumped(failed)
	}
	return n
}

// Run drives Tick every interval until ctx is cancelled. It only ticks while
// enabled; tick errors are logged and never stop the loop (supervision must
// survive transient failures). interval<=0 uses defaultInterval. The wiring
// starts this in a goroutine.
func (a *Autopilot) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = defaultInterval
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if a.Enabled() {
				if err := a.Tick(ctx); err != nil {
					log.Printf("autopilot: tick: %v", err)
				}
			}
		}
	}
}
