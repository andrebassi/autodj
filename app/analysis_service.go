package app

import (
	"context"
	"log"
	"sync"

	"autodj/ports/outbound"
)

// AnalysisService runs BPM/key/waveform analysis off the ingest hot-path. It
// owns a worker pool draining a bounded queue of track ids; each job loads the
// track, skips non-PCM sources, runs the analyzer, applies the result in the
// domain, and re-upserts. Analyzer failures are logged and never corrupt the
// stored track. It implements AnalysisEnqueuer.
type AnalysisService struct {
	repo     outbound.TrackRepo
	analyzer outbound.AudioAnalyzer
	workers  int
	jobs     chan string
	wg       sync.WaitGroup

	mu      sync.Mutex
	started bool
	stopped bool
}

// NewAnalysisService builds the service. workers < 1 is clamped to 1; a queue
// buffer keeps ingest non-blocking under bursts.
func NewAnalysisService(repo outbound.TrackRepo, analyzer outbound.AudioAnalyzer, workers int) *AnalysisService {
	if workers < 1 {
		workers = 1
	}
	return &AnalysisService{
		repo:     repo,
		analyzer: analyzer,
		workers:  workers,
		jobs:     make(chan string, 256),
	}
}

// Start launches the worker pool. It is idempotent. The pool drains until Stop
// closes the queue or ctx is cancelled.
func (s *AnalysisService) Start(ctx context.Context) {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return
	}
	s.started = true
	s.mu.Unlock()

	for i := 0; i < s.workers; i++ {
		s.wg.Add(1)
		go s.worker(ctx)
	}
}

func (s *AnalysisService) worker(ctx context.Context) {
	defer s.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case id, ok := <-s.jobs:
			if !ok {
				return
			}
			if err := s.analyzeOne(ctx, id); err != nil {
				log.Printf("analysis: track %s: %v", id, err)
			}
		}
	}
}

// Enqueue schedules a track id for analysis. It never blocks the caller: if the
// queue is full or the service is stopped, the job is dropped (best-effort;
// boot backfill re-enqueues un-analyzed tracks next start).
func (s *AnalysisService) Enqueue(id string) {
	if id == "" {
		return
	}
	s.mu.Lock()
	stopped := s.stopped
	s.mu.Unlock()
	if stopped {
		return
	}
	select {
	case s.jobs <- id:
	default:
		log.Printf("analysis: queue full, dropped %s", id)
	}
}

// analyzeOne loads a track, analyzes it if PCM, and re-upserts. Non-PCM tracks
// are skipped silently (nil). A failed Get/Analyze/Upsert returns the error for
// the worker to log; the stored track is left untouched on analyzer failure.
func (s *AnalysisService) analyzeOne(ctx context.Context, id string) error {
	t, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	if !t.PCMAvailable {
		return nil // DRM/non-PCM: nothing to analyze
	}
	a, err := s.analyzer.Analyze(ctx, t.Path)
	if err != nil {
		return err
	}
	if err := t.ApplyAnalysis(a); err != nil {
		return err
	}
	return s.repo.Upsert(ctx, t)
}

// Backfill enqueues every existing PCM track still missing analysis, so a
// library persisted before a given measurement existed gets filled in on boot
// without re-uploading. Errors listing are returned; individual enqueues are
// best-effort.
//
// A faixa entra na fila quando falta QUALQUER uma das medidas — BPM (o critério
// antigo), loudness ou imagem estéreo. Só `!Analyzed()` não bastava: acervo que
// já tinha BPM ficaria PARA SEMPRE sem as outras, ou seja, o ganho constante e o
// modo VOZ nasceriam funcionando só para faixa nova. É a diferença entre
// "estreou" e "estreou para quem já usa o app".
//
// Por isso a análise grava o PISO na faixa mono em vez de deixar 0: mono medido
// é resposta, e uma faixa carimbada com 0 voltaria pra esta fila em todo boot.
func (s *AnalysisService) Backfill(ctx context.Context) (int, error) {
	tracks, err := s.repo.List(ctx)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, t := range tracks {
		if !t.PCMAvailable {
			continue
		}
		if t.Analyzed() && t.LoudnessLUFS != 0 {
			continue
		}
		s.Enqueue(t.ID)
		n++
	}
	return n, nil
}

// Stop closes the queue and waits for in-flight jobs to drain. It is idempotent.
func (s *AnalysisService) Stop() {
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

var _ AnalysisEnqueuer = (*AnalysisService)(nil)
