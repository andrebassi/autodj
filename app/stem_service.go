package app

import (
	"context"
	"errors"
	"log"
	"path/filepath"
	"strings"
	"sync"

	"autodj/domain/library"
	"autodj/ports/outbound"
)

var (
	// ErrSeparatorUnavailable is returned when AI separation is not installed on
	// this machine. A UI que não puder oferecer o recurso some com ele, em vez de
	// falhar no clique — mesma degradação graciosa do analisador.
	ErrSeparatorUnavailable = errors.New("stem: separação por IA indisponível nesta máquina")
	// ErrNotSeparable is returned for a track with no PCM on our side
	// (Spotify/YouTube tocam out-of-band: não há arquivo para alimentar o modelo).
	ErrNotSeparable = errors.New("stem: faixa sem PCM local (streaming não separa)")
)

// StemStatus is what the UI shows for one track.
//
// ALIAS de string, não tipo novo: a porta inbound fala em `string` (ela não pode
// importar `app`), e um tipo distinto obrigaria conversões em toda fronteira sem
// comprar segurança nenhuma — os valores já são um conjunto fechado documentado
// aqui.
type StemStatus = string

const (
	StemIdle    StemStatus = "idle"    // nunca pedido (ou faixa sem preparo)
	StemQueued  StemStatus = "queued"  // na fila do worker
	StemRunning StemStatus = "running" // modelo rodando
	StemReady   StemStatus = "ready"   // arquivo pronto
	StemFailed  StemStatus = "failed"  // tentou e falhou
)

/*
StemService prepara o INSTRUMENTAL de faixas locais com separação por IA.

É o único caminho que resolve faixa MONO — o modo VOZ (cancelamento de centro)
precisa de imagem estéreo, o modelo não.

⚠️ FILA DE UM TRABALHADOR SÓ, de propósito. A separação leva ~1,4× o tempo real
(medido: 45s de áudio em 64,8s num M1 Pro com 6 threads) e já usa vários núcleos
por dentro. Dois trabalhos em paralelo não terminariam mais rápido — só
disputariam CPU com o playout, que roda ao mesmo tempo e não pode engasgar.

Por isso também NUNCA é chamado no caminho do deck: é pré-render, guardado em
arquivo, pedido explicitamente pelo operador.
*/
type StemService struct {
	repo outbound.TrackRepo
	sep  outbound.StemSeparator
	// dir é onde os instrumentais são escritos. Fica FORA do diretório de mídia do
	// operador de propósito: é arquivo derivado, recriável, e não deve aparecer
	// como faixa se alguém varrer a pasta da biblioteca.
	dir string

	jobs chan string
	wg   sync.WaitGroup

	mu      sync.Mutex
	status  map[string]StemStatus
	errs    map[string]string
	started bool
	stopped bool
}

// NewStemService wires the service. dir é onde os arquivos gerados ficam.
func NewStemService(repo outbound.TrackRepo, sep outbound.StemSeparator, dir string) *StemService {
	return &StemService{
		repo:   repo,
		sep:    sep,
		dir:    dir,
		jobs:   make(chan string, 128),
		status: map[string]StemStatus{},
		errs:   map[string]string{},
	}
}

// Available reports whether AI separation can run at all on this machine.
func (s *StemService) Available() bool { return s.sep != nil && s.sep.Available() }

// Start launches the single worker. Idempotente.
func (s *StemService) Start(ctx context.Context) {
	s.mu.Lock()
	if s.started || !s.Available() {
		s.mu.Unlock()
		return
	}
	s.started = true
	s.mu.Unlock()
	s.wg.Add(1)
	go s.worker(ctx)
}

func (s *StemService) worker(ctx context.Context) {
	defer s.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case id, ok := <-s.jobs:
			if !ok {
				return
			}
			s.run(ctx, id)
		}
	}
}

// Status devolve o estado de uma faixa (e o motivo, quando falhou).
func (s *StemService) Status(ctx context.Context, id string) (StemStatus, string) {
	s.mu.Lock()
	st, ok := s.status[id]
	msg := s.errs[id]
	s.mu.Unlock()
	if ok {
		return st, msg
	}
	// Sem estado em memória (reinício do servidor): o ARQUIVO é a verdade. Um
	// instrumental preparado ontem continua pronto hoje.
	if t, err := s.repo.Get(ctx, id); err == nil && t.HasInstrumental() {
		return StemReady, ""
	}
	return StemIdle, ""
}

/*
Working devolve o estado de separação de todas as faixas que TÊM algum: na fila,
rodando ou já prontas. Por id.

Existe para as LISTAS: perguntar faixa a faixa daria uma requisição por linha, e o
acervo tem dezenas.

⚠️ `ready` ENTRA AQUI, e isso mudou de ideia por um motivo concreto. A primeira
versão devolvia só trabalho vivo, com o argumento de que `ready` já viajava no
`trackDTO` de cada faixa. Verdade para a GRADE de mídias — mas a FILA de execução
não é montada de `trackDTO`: ela vem do `Playable`, que é um retrato do instante
do enfileiramento e por isso não pode carregar o estado do preparo (ele termina
minutos depois). Sem `ready` aqui, a fila não teria como marcar as faixas prontas.

Custa pouco: só faixa preparada entra, e são poucas.

`idle` e `failed` ficam de fora — nada a desenhar. Mapa vazio significa "nada
preparado e nada em andamento", e é isso que faz o poll do cliente parar.
*/
func (s *StemService) Working(ctx context.Context) map[string]StemStatus {
	out := map[string]StemStatus{}
	s.mu.Lock()
	for id, st := range s.status {
		if st == StemQueued || st == StemRunning {
			out[id] = st
		}
	}
	s.mu.Unlock()
	// as prontas saem do REPO, não da memória: o estado em memória se perde no
	// restart do servidor, e o arquivo é a verdade.
	if tracks, err := s.repo.List(ctx); err == nil {
		for _, t := range tracks {
			if _, busy := out[t.ID]; busy {
				continue // em andamento manda mais que o arquivo antigo
			}
			if t.HasInstrumental() {
				out[t.ID] = StemReady
			}
		}
	}
	return out
}

/*
PrepareAll enfileira TODA faixa local que ainda não tem instrumental. Devolve
quantas entraram.

É o que faz o interruptor "preparar automaticamente" valer para o acervo que já
existe, e não só para faixa nova. Sem isto, ligar a opção com 83 músicas na
biblioteca não produziria nada visível — o operador concluiria que não funciona.

⚠️ ISTO COMPROMETE A MÁQUINA POR HORAS: ~1,4× o tempo real por faixa, uma de cada
vez. 80 músicas ≈ 5 h de CPU. Por isso só é chamado quando o operador LIGA a opção
(ou no boot com ela já ligada) — nunca por conta própria.

Para quando a fila enche (o canal tem teto): o que sobrou fica de fora e é
registrado. Insistir bloquearia o chamador — que é uma requisição HTTP — e o resto
entra na próxima vez que a opção for ligada ou o servidor subir.
*/
func (s *StemService) PrepareAll(ctx context.Context) int {
	if !s.Available() {
		return 0
	}
	tracks, err := s.repo.List(ctx)
	if err != nil {
		return 0
	}
	n := 0
	for _, t := range tracks {
		if !t.Separable() || t.HasInstrumental() {
			continue
		}
		/* ⚠️ CONSULTA ANTES DE PEDIR, senão a CONTA mente. `Prepare` devolve
		   `queued` tanto para "acabei de enfileirar" quanto para "já estava na
		   fila" — ele não distingue, e nem deveria: do ponto de vista de quem
		   pede, o resultado é o mesmo. Quem precisa da diferença é este laço, que
		   informa quantas ENTRARAM. Sem isto, chamar duas vezes seguidas contava
		   as mesmas faixas de novo. */
		if cur, _ := s.Status(ctx, t.ID); cur == StemQueued || cur == StemRunning {
			continue
		}
		st, err := s.Prepare(ctx, t.ID)
		if err != nil {
			log.Printf("stem: preparo em lote parou em %q (%d enfileiradas): %v", t.Title, n, err)
			break
		}
		if st == StemQueued {
			n++
		}
	}
	if n > 0 {
		log.Printf("stem: %d faixa(s) enfileirada(s) para separação em lote", n)
	}
	return n
}

func (s *StemService) set(id string, st StemStatus, msg string) {
	s.mu.Lock()
	s.status[id] = st
	if msg == "" {
		delete(s.errs, id)
	} else {
		s.errs[id] = msg
	}
	s.mu.Unlock()
}

/*
Prepare enfileira a separação de uma faixa. Devolve o estado resultante.

Idempotente por desenho: já pronto devolve `ready` sem refazer (são minutos de
CPU), e já na fila/rodando não duplica o trabalho.
*/
func (s *StemService) Prepare(ctx context.Context, id string) (StemStatus, error) {
	if !s.Available() {
		return StemIdle, ErrSeparatorUnavailable
	}
	t, err := s.repo.Get(ctx, id)
	if err != nil {
		return StemIdle, err
	}
	if !t.Separable() {
		return StemIdle, ErrNotSeparable
	}
	if t.HasInstrumental() {
		s.set(id, StemReady, "")
		return StemReady, nil
	}
	s.mu.Lock()
	cur := s.status[id]
	stopped := s.stopped
	s.mu.Unlock()
	if stopped {
		return StemIdle, ErrSeparatorUnavailable
	}
	if cur == StemQueued || cur == StemRunning {
		return cur, nil // já em andamento: não duplica minutos de CPU
	}
	select {
	case s.jobs <- id:
		s.set(id, StemQueued, "")
		return StemQueued, nil
	default:
		return StemIdle, errors.New("stem: fila cheia, tente daqui a pouco")
	}
}

// run executa a separação e grava o caminho na faixa.
func (s *StemService) run(ctx context.Context, id string) {
	t, err := s.repo.Get(ctx, id)
	if err != nil {
		s.set(id, StemFailed, err.Error())
		return
	}
	s.set(id, StemRunning, "")
	out := s.outPath(t)
	if err := s.sep.SeparateVocals(ctx, t.Path, out); err != nil {
		log.Printf("stem: %s: %v", id, err)
		s.set(id, StemFailed, err.Error())
		return
	}
	t.InstrumentalPath = out
	if err := s.repo.Upsert(ctx, t); err != nil {
		log.Printf("stem: upsert %s: %v", id, err)
		s.set(id, StemFailed, err.Error())
		return
	}
	s.set(id, StemReady, "")
}

// outPath deriva o nome do arquivo gerado a partir do ID da faixa (e não do nome
// original): o id é único e já saneado, enquanto o título do operador pode ter
// barra, acento e dois arquivos com o mesmo nome.
func (s *StemService) outPath(t *library.Track) string {
	name := strings.ReplaceAll(t.ID, string(filepath.Separator), "_") + ".instrumental.wav"
	return filepath.Join(s.dir, name)
}

// Stop drains the queue and waits. Idempotente.
func (s *StemService) Stop() {
	s.mu.Lock()
	if s.stopped || !s.started {
		s.stopped = true
		s.mu.Unlock()
		return
	}
	s.stopped = true
	close(s.jobs)
	s.mu.Unlock()
	s.wg.Wait()
}
