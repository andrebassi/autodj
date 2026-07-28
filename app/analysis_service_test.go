package app

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"autodj/adapters/driven/memory"
	"autodj/domain/library"
	"autodj/domain/shared"
	"autodj/ports/outbound"
)

// fakeAnalyzer returns a fixed Analysis (or error) and counts calls.
type fakeAnalyzer struct {
	out  library.Analysis
	err  error
	mu   sync.Mutex
	seen []string
}

func (f *fakeAnalyzer) Analyze(_ context.Context, path string) (library.Analysis, error) {
	f.mu.Lock()
	f.seen = append(f.seen, path)
	f.mu.Unlock()
	return f.out, f.err
}

func (f *fakeAnalyzer) calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.seen)
}

func seedLocal(t *testing.T, repo *memory.TrackRepo, id, path string) *library.Track {
	t.Helper()
	tr, err := library.NewTrack(id, shared.SourceLocal, "Neon Dusk", 3*time.Minute)
	if err != nil {
		t.Fatalf("NewTrack: %v", err)
	}
	tr.Path = path
	tr.Category = "music"
	if err := repo.Upsert(context.Background(), tr); err != nil {
		t.Fatalf("seed upsert: %v", err)
	}
	return tr
}

func TestAnalyzeOnePCM(t *testing.T) {
	repo := memory.New()
	seedLocal(t, repo, "id1", "/music/a.flac")
	az := &fakeAnalyzer{out: library.Analysis{BPM: 128, MusicalKey: "8A", WaveformRef: "peaks/id1.json"}}
	svc := NewAnalysisService(repo, az, 2)

	if err := svc.analyzeOne(context.Background(), "id1"); err != nil {
		t.Fatalf("analyzeOne: %v", err)
	}
	got, _ := repo.Get(context.Background(), "id1")
	if got.BPM != 128 || got.MusicalKey != "8A" || !got.Analyzed() {
		t.Fatalf("analysis not persisted: %+v", got)
	}
}

func TestAnalyzeOneSkipsNonPCM(t *testing.T) {
	repo := memory.New()
	sp, err := library.NewTrack("sp1", shared.SourceSpotify, "DRM", 3*time.Minute)
	if err != nil {
		t.Fatalf("NewTrack: %v", err)
	}
	sp.SpotifyURI = "spotify:track:x"
	if err := repo.Upsert(context.Background(), sp); err != nil {
		t.Fatalf("seed: %v", err)
	}
	az := &fakeAnalyzer{out: library.Analysis{BPM: 120}}
	svc := NewAnalysisService(repo, az, 1)

	if err := svc.analyzeOne(context.Background(), "sp1"); err != nil {
		t.Fatalf("want nil (skip), got %v", err)
	}
	if az.calls() != 0 {
		t.Fatalf("analyzer must not be called for non-PCM, calls=%d", az.calls())
	}
	got, _ := repo.Get(context.Background(), "sp1")
	if got.BPM != 0 {
		t.Fatalf("spotify gained BPM: %v", got.BPM)
	}
}

func TestAnalyzeOneNotFound(t *testing.T) {
	repo := memory.New()
	svc := NewAnalysisService(repo, &fakeAnalyzer{}, 1)
	if err := svc.analyzeOne(context.Background(), "missing"); err == nil {
		t.Fatal("want error for missing id")
	}
}

func TestAnalyzeOneAnalyzerErrorLeavesTrackIntact(t *testing.T) {
	repo := memory.New()
	seedLocal(t, repo, "id1", "/music/a.flac")
	az := &fakeAnalyzer{err: errors.New("aubio crashed")}
	svc := NewAnalysisService(repo, az, 1)

	if err := svc.analyzeOne(context.Background(), "id1"); err == nil {
		t.Fatal("want analyzer error")
	}
	got, _ := repo.Get(context.Background(), "id1")
	if got.BPM != 0 {
		t.Fatalf("track corrupted after analyzer failure: %+v", got)
	}
}

func TestStartEnqueueStopDrains(t *testing.T) {
	repo := memory.New()
	for _, id := range []string{"id1", "id2", "id3"} {
		seedLocal(t, repo, id, "/music/"+id+".flac")
	}
	az := &fakeAnalyzer{out: library.Analysis{BPM: 126, MusicalKey: "9A"}}
	svc := NewAnalysisService(repo, az, 3)

	svc.Start(context.Background())
	svc.Start(context.Background()) // idempotent
	svc.Enqueue("id1")
	svc.Enqueue("id2")
	svc.Enqueue("id3")
	svc.Enqueue("") // ignored
	svc.Stop()      // closes queue, waits for drain

	for _, id := range []string{"id1", "id2", "id3"} {
		got, _ := repo.Get(context.Background(), id)
		if !got.Analyzed() || got.BPM != 126 {
			t.Fatalf("track %s not analyzed after drain: %+v", id, got)
		}
	}
	// Enqueue after stop is a no-op (no panic on closed channel).
	svc.Enqueue("id1")
	svc.Stop() // idempotent
}

func TestBackfillEnqueuesUnanalyzedPCM(t *testing.T) {
	repo := memory.New()
	// Un-analyzed PCM → should enqueue.
	seedLocal(t, repo, "id1", "/music/a.flac")
	// Fully analyzed PCM (BPM e loudness) → skip.
	done := seedLocal(t, repo, "id2", "/music/b.flac")
	done.BPM = 128
	done.LoudnessLUFS = -11.2
	_ = repo.Upsert(context.Background(), done)
	// Non-PCM → skip.
	sp, _ := library.NewTrack("sp1", shared.SourceSpotify, "DRM", 3*time.Minute)
	sp.SpotifyURI = "spotify:track:x"
	_ = repo.Upsert(context.Background(), sp)

	az := &fakeAnalyzer{out: library.Analysis{BPM: 130}}
	svc := NewAnalysisService(repo, az, 2)
	svc.Start(context.Background())
	n, err := svc.Backfill(context.Background())
	if err != nil {
		t.Fatalf("Backfill: %v", err)
	}
	if n != 1 {
		t.Fatalf("want 1 backfilled (only un-analyzed PCM), got %d", n)
	}
	svc.Stop()
	got, _ := repo.Get(context.Background(), "id1")
	if !got.Analyzed() {
		t.Fatalf("backfilled track not analyzed: %+v", got)
	}
}

// Faixa com BPM mas SEM loudness volta pra fila. Sem isto, o acervo que já
// existia nunca ganharia a medida nova — o ganho constante estrearia só para
// música enviada dali em diante, que é o mesmo que não estrear.
func TestBackfillReenqueuesTrackMissingLoudness(t *testing.T) {
	repo := memory.New()
	old := seedLocal(t, repo, "id1", "/music/a.flac")
	old.BPM = 128 // analisada pelo critério antigo…
	old.LoudnessLUFS = 0
	_ = repo.Upsert(context.Background(), old)

	az := &fakeAnalyzer{out: library.Analysis{BPM: 128, LoudnessLUFS: -13.4}}
	svc := NewAnalysisService(repo, az, 1)
	svc.Start(context.Background())
	n, err := svc.Backfill(context.Background())
	if err != nil {
		t.Fatalf("Backfill: %v", err)
	}
	if n != 1 {
		t.Fatalf("want 1 re-enfileirada, got %d", n)
	}
	svc.Stop()
	got, _ := repo.Get(context.Background(), "id1")
	if got.LoudnessLUFS != -13.4 {
		t.Fatalf("loudness não foi preenchido: %+v", got)
	}
}

func TestNewAnalysisServiceClampsWorkers(t *testing.T) {
	svc := NewAnalysisService(memory.New(), &fakeAnalyzer{}, 0)
	if svc.workers != 1 {
		t.Fatalf("workers should clamp to 1, got %d", svc.workers)
	}
}

func TestStopBeforeStart(t *testing.T) {
	svc := NewAnalysisService(memory.New(), &fakeAnalyzer{}, 1)
	svc.Stop() // must not panic / deadlock when never started
	svc.Enqueue("x")
}

// enqueuer wiring: LibraryService enqueues PCM upserts, skips non-PCM.
func TestLibraryServiceEnqueuesOnScan(t *testing.T) {
	repo := memory.New()
	scan := fakeScanner{paths: []string{"/a.mp3"}}
	meta := fakeMeta{byPath: map[string]outbound.Metadata{"/a.mp3": {Title: "A", Duration: 2 * time.Minute}}}
	svc := NewLibraryService(repo, scan, meta, nil)
	fe := &fakeEnqueuer{}
	svc.SetAnalysisEnqueuer(fe)
	if _, err := svc.ScanFolder(context.Background(), "/root"); err != nil {
		t.Fatalf("ScanFolder: %v", err)
	}
	if len(fe.ids) != 1 {
		t.Fatalf("want 1 enqueued PCM track, got %d", len(fe.ids))
	}
}

/*
Faixa nova tem de AVISAR o piloto na hora.

Sem este aviso, o piloto só reagiria no próximo tick — até 15 segundos. Com a fila
vazia, o operador adiciona música e a estação fica muda nesse intervalo, o que é
indistinguível de travamento.
*/
func TestLibraryServiceAvisaOAcervoMudou(t *testing.T) {
	repo := memory.New()
	scan := fakeScanner{paths: []string{"/a.mp3"}}
	meta := fakeMeta{byPath: map[string]outbound.Metadata{"/a.mp3": {Title: "A", Duration: 2 * time.Minute}}}
	svc := NewLibraryService(repo, scan, meta, nil)
	w := &fakeWatcher{}
	svc.SetCatalogWatcher(w)
	if _, err := svc.ScanFolder(context.Background(), "/root"); err != nil {
		t.Fatalf("ScanFolder: %v", err)
	}
	if w.calls() == 0 {
		t.Fatal("acervo ganhou faixa e ninguém avisou o piloto")
	}
}

type fakeWatcher struct {
	mu sync.Mutex
	n  int
}

func (f *fakeWatcher) CatalogChanged() {
	f.mu.Lock()
	f.n++
	f.mu.Unlock()
}

func (f *fakeWatcher) calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.n
}

type fakeEnqueuer struct {
	mu  sync.Mutex
	ids []string
}

func (f *fakeEnqueuer) Enqueue(id string) {
	f.mu.Lock()
	f.ids = append(f.ids, id)
	f.mu.Unlock()
}
