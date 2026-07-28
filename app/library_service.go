// Package app holds the application services: they implement the inbound
// use-case ports by orchestrating the domain and the outbound ports. They hold
// no business rules of their own (those live in domain) and no I/O (that lives
// in adapters) — just wiring and transaction-of-work.
package app

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"log"

	"autodj/domain/library"
	"autodj/domain/shared"
	"autodj/ports/inbound"
	"autodj/ports/outbound"
)

// AnalysisEnqueuer schedules a track for async BPM/key/waveform analysis. It is
// implemented by AnalysisService; kept as a local interface so LibraryService
// stays decoupled and testable with a fake.
type AnalysisEnqueuer interface {
	Enqueue(id string)
}

/*
CatalogWatcher é avisado quando o acervo GANHA faixa (upload, scan, import).

Existe por um motivo medido: o piloto supervisiona a cada 15s, então adicionar
música com a fila vazia deixava a estação muda por até 15 segundos. Do lado do
operador isso é indistinguível de travamento — ele adiciona e não acontece nada.

É implementado pelo Autopilot (`Tick`), mas fica como interface local pela mesma
razão do AnalysisEnqueuer: o LibraryService não conhece o piloto, e o teste usa um
fake. Opcional — nil só significa "ninguém escutando".
*/
type CatalogWatcher interface {
	CatalogChanged()
}

// LibraryService implements inbound.LibraryService.
type LibraryService struct {
	repo     outbound.TrackRepo
	scanner  outbound.FolderScanner
	meta     outbound.MetadataExtractor
	media    outbound.MediaStore
	analysis AnalysisEnqueuer          // optional; nil disables async analysis
	waveform outbound.WaveformGenerator // optional; nil disables waveform
	folders  outbound.FolderCatalog     // optional; nil = sem pastas vazias persistidas
	watcher  CatalogWatcher             // optional; nil = ninguém escuta o acervo
	stems    StemQueue                  // optional; nil = sem preparo automático
	mix      outbound.MixSettingsRepo           // optional; lê o interruptor do preparo
}

/*
StemQueue é o mínimo que a biblioteca precisa do serviço de separação: enfileirar.

Interface própria (e não o `StemService` inteiro) pelo mesmo motivo do
`AnalysisEnqueuer`: quem importa uma faixa não tem nada a ver com estado, fila ou
erro da separação — só avisa que existe faixa nova. Menor superfície, e o teste
da biblioteca não precisa montar um separador.
*/
type StemQueue interface {
	Prepare(ctx context.Context, id string) (string, error)
}

// SetStemQueue liga o preparo automático de instrumental. Sem isto (ou sem o
// MixRepo), toda faixa continua dependendo do pedido manual no menu.
func (s *LibraryService) SetStemQueue(q StemQueue, mix outbound.MixSettingsRepo) {
	s.stems = q
	s.mix = mix
}

// NewLibraryService wires the library use cases. media may be nil when uploads
// are not configured (Upload then returns ErrUploadUnavailable).
func NewLibraryService(repo outbound.TrackRepo, scanner outbound.FolderScanner, meta outbound.MetadataExtractor, media outbound.MediaStore) *LibraryService {
	return &LibraryService{repo: repo, scanner: scanner, meta: meta, media: media}
}

// SetAnalysisEnqueuer wires an optional analysis enqueuer. When set, successful
// upserts of PCM tracks (scan/upload) are queued for async analysis.
func (s *LibraryService) SetAnalysisEnqueuer(a AnalysisEnqueuer) { s.analysis = a }

// SetCatalogWatcher wires an optional listener notified when the catalog gains a
// track. O piloto usa isso pra reagir NA HORA, em vez de no próximo tick de 15s.
func (s *LibraryService) SetCatalogWatcher(w CatalogWatcher) { s.watcher = w }

// enqueueAnalysis schedules a PCM track for analysis when an enqueuer is wired.
// Non-PCM (DRM) tracks are never analyzable and are skipped.
// Também avisa o observador do acervo: faixa nova precisa poder entrar na fila
// imediatamente, e não daqui a até 15 segundos.
func (s *LibraryService) enqueueAnalysis(t *library.Track) {
	if s.analysis != nil && t.PCMAvailable {
		s.analysis.Enqueue(t.ID)
	}
	s.autoInstrumental(t)
	if s.watcher != nil {
		s.watcher.CatalogChanged()
	}
}

/*
autoInstrumental enfileira a separação da faixa recém-chegada, quando o operador
ligou isso em ⚙ Configurações.

Best-effort de propósito, e cada silêncio aqui é deliberado:
  - sem separador ou sem repo de configuração, não faz nada (a máquina não tem a
    ferramenta — é o caso de produção, onde o recurso nem existe);
  - erro de leitura da configuração ou da fila NÃO derruba o import. Uma faixa que
    entrou no acervo mas não ganhou instrumental continua tocando; abortar o
    upload por causa de um extra seria trocar o essencial pelo acessório;
  - `Prepare` já é idempotente (pronto devolve pronto, em andamento não duplica),
    então re-importar o mesmo arquivo não custa CPU de novo.
*/
func (s *LibraryService) autoInstrumental(t *library.Track) {
	if s.stems == nil || s.mix == nil || !t.Separable() || t.HasInstrumental() {
		return
	}
	ctx := context.Background()
	cfg, err := s.mix.Get(ctx)
	if err != nil || !cfg.AutoInstrumental {
		return
	}
	if _, err := s.stems.Prepare(ctx, t.ID); err != nil {
		// fila cheia (o canal tem teto) ou ferramenta ausente: registra e segue.
		// O operador ainda pode pedir faixa a faixa pelo menu.
		log.Printf("stem: preparo automático de %q não entrou na fila: %v", t.Title, err)
	}
}

// ErrUploadUnavailable is returned by Upload when no MediaStore is wired.
var ErrUploadUnavailable = errors.New("library: upload not available (no media store)")

// ErrWaveformUnavailable is returned by Waveform when no WaveformGenerator is
// wired.
var ErrWaveformUnavailable = errors.New("library: waveform not available (no generator)")

// SetWaveformGenerator wires an optional peak generator. When set, Waveform
// serves peaks for PCM tracks; otherwise it returns ErrWaveformUnavailable.
func (s *LibraryService) SetWaveformGenerator(g outbound.WaveformGenerator) { s.waveform = g }

// Waveform returns the peaks + duration for the waveform editor. It loads the
// track (ErrTrackNotFound if absent), enforces the two-engine invariant
// (library.ErrWaveformNotPCM for a DRM source — peaks require PCM), then
// delegates to the generator. Without a generator wired it returns
// ErrWaveformUnavailable.
func (s *LibraryService) Waveform(ctx context.Context, id string, bars int) (inbound.WaveformData, error) {
	if s.waveform == nil {
		return inbound.WaveformData{}, ErrWaveformUnavailable
	}
	t, err := s.repo.Get(ctx, id)
	if err != nil {
		return inbound.WaveformData{}, library.ErrTrackNotFound
	}
	if !t.PCMAvailable {
		return inbound.WaveformData{}, library.ErrWaveformNotPCM
	}
	peaks, err := s.waveform.Peaks(ctx, t, bars)
	if err != nil {
		return inbound.WaveformData{}, err
	}
	return inbound.WaveformData{DurationMs: t.Duration.Milliseconds(), Peaks: peaks}, nil
}

// idFor derives a stable id from a file path so re-scans upsert instead of
// duplicating.
func idFor(path string) string {
	sum := sha1.Sum([]byte(path))
	return hex.EncodeToString(sum[:])
}

// ScanFolder scans root, extracts metadata, and upserts local Tracks. Files
// that fail extraction or validation are skipped (scan is best-effort).
func (s *LibraryService) ScanFolder(ctx context.Context, root string) (int, error) {
	paths, err := s.scanner.Scan(ctx, root)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, p := range paths {
		md, err := s.meta.Extract(ctx, p)
		if err != nil {
			continue
		}
		t, err := library.NewTrack(idFor(p), shared.SourceLocal, md.Title, md.Duration)
		if err != nil {
			continue
		}
		t.Artist = md.Artist
		t.Album = md.Album
		t.Path = p
		t.Category = "music"
		if md.Title == "" {
			t.Title = p
		}
		if err := t.Validate(); err != nil {
			continue
		}
		if err := s.repo.Upsert(ctx, t); err != nil {
			return n, err
		}
		s.enqueueAnalysis(t)
		n++
	}
	return n, nil
}

// Tracks lists the whole catalog.
func (s *LibraryService) Tracks(ctx context.Context) ([]*library.Track, error) {
	return s.repo.List(ctx)
}

// Categories returns the fixed set of valid media categories.
func (s *LibraryService) Categories() []library.Category { return library.Categories() }

// Upload saves an uploaded file via the MediaStore, probes it, and creates a
// local (PCM) Track. The two-engine invariant holds: uploads are always local,
// never Spotify, so mixable stays derived from source.
func (s *LibraryService) Upload(ctx context.Context, in inbound.UploadInput) (*library.Track, error) {
	if s.media == nil {
		return nil, ErrUploadUnavailable
	}
	path, err := s.media.Save(ctx, in.Filename, in.Content)
	if err != nil {
		return nil, err
	}
	md, err := s.meta.Extract(ctx, path)
	if err != nil {
		// Probe failed: undo the saved file so we don't leak an orphan.
		_ = s.media.Remove(ctx, path)
		return nil, err
	}
	t, err := library.NewTrack(idFor(path), shared.SourceLocal, md.Title, md.Duration)
	if err != nil {
		_ = s.media.Remove(ctx, path)
		return nil, err
	}
	t.Path = path
	t.Artist = md.Artist
	t.Album = md.Album
	t.Category = in.Category
	// Operator-supplied metadata overrides the probed values when set.
	if in.Title != "" {
		t.Title = in.Title
	} else if t.Title == "" {
		t.Title = in.Filename
	}
	if in.Artist != "" {
		t.Artist = in.Artist
	}
	if in.Album != "" {
		t.Album = in.Album
	}
	// Pastas do envio (valem pro lote inteiro). Validate normaliza e recusa nome
	// inválido — melhor o upload falhar do que gravar pasta suja no acervo.
	t.Folders = in.Folders
	if err := t.Validate(); err != nil {
		_ = s.media.Remove(ctx, path)
		return nil, err
	}
	if err := s.repo.Upsert(ctx, t); err != nil {
		_ = s.media.Remove(ctx, path)
		return nil, err
	}
	// pasta usada no envio entra no catálogo: continua existindo se a faixa sair
	if s.folders != nil {
		for _, f := range t.Folders {
			_ = s.folders.Create(ctx, f)
		}
	}
	s.enqueueAnalysis(t)
	return t, nil
}

// UpdateTrack edits only the metadata (title/artist/album/category) of an
// existing track. Source/path/duration/flags are never touched.
func (s *LibraryService) UpdateTrack(ctx context.Context, id string, in inbound.UpdateInput) (*library.Track, error) {
	t, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, library.ErrTrackNotFound
	}
	t.Title = in.Title
	t.Artist = in.Artist
	t.Album = in.Album
	t.Category = in.Category
	if err := t.Validate(); err != nil {
		return nil, err
	}
	if err := s.repo.Upsert(ctx, t); err != nil {
		return nil, err
	}
	return t, nil
}

// SetMarkers replaces a track's cue markers. It loads the track, applies the
// markers in the domain (which enforces PCM-only and validity), and re-upserts.
// Metadata and flags are untouched.
func (s *LibraryService) SetMarkers(ctx context.Context, id string, markers []library.Marker) (*library.Track, error) {
	t, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, library.ErrTrackNotFound
	}
	if err := t.SetMarkers(markers); err != nil {
		return nil, err
	}
	if err := s.repo.Upsert(ctx, t); err != nil {
		return nil, err
	}
	return t, nil
}

// Audio resolves the local file path of a track's raw audio for the DJ deck /
// cartwall to fetch and decode in the browser. It enforces the two-engine
// invariant at the serving edge: only Mixable && PCMAvailable tracks yield
// bytes — a Spotify (DRM) track returns ErrTrackNotMixable and never touches
// disk. A mixable track with no path returns ErrNoAudioPath.
func (s *LibraryService) Audio(ctx context.Context, id string) (*library.Track, error) {
	t, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, library.ErrTrackNotFound
	}
	if !t.Mixable || !t.PCMAvailable {
		return nil, library.ErrTrackNotMixable
	}
	if t.Path == "" {
		return nil, library.ErrNoAudioPath
	}
	return t, nil
}

// Artwork resolves a track's cover so the delivery layer can serve it, uniform
// across providers (hexagonal): the same call answers for local, Spotify or any
// future source. It returns the full Track — the delivery layer redirects to
// ArtworkURL when set (external cover) or extracts the embedded picture from the
// local file. ErrTrackNotFound if absent; ErrNoArtwork when a track has neither
// an external URL nor a local file to probe (→ 404 → UI placeholder).
func (s *LibraryService) Artwork(ctx context.Context, id string) (*library.Track, error) {
	t, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, library.ErrTrackNotFound
	}
	// No cover source at all → 404 up front. A YouTube track carries neither a
	// stored ArtworkURL (older imports) nor a local Path, but its YouTubeURI lets
	// the HTTP handler derive the public ytimg thumbnail — so keep it alive here.
	if t.ArtworkURL == "" && t.Path == "" && t.YouTubeURI == "" {
		return nil, library.ErrNoArtwork
	}
	return t, nil
}

// DeleteTrack removes a track from the catalog then best-effort deletes its
// media file. The repo is the source of truth; a missing/undeletable file (e.g.
// the demo seed's fictional path) does not fail the delete.
func (s *LibraryService) DeleteTrack(ctx context.Context, id string) error {
	t, err := s.repo.Get(ctx, id)
	if err != nil {
		return library.ErrTrackNotFound
	}
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	if s.media != nil && t.Path != "" {
		_ = s.media.Remove(ctx, t.Path)
	}
	return nil
}

// ClearLibrary wipes the whole catalog (header "Limpar" → começar do zero):
// removes every track and best-effort deletes their media files. Idempotent.
func (s *LibraryService) ClearLibrary(ctx context.Context) error {
	if s.media != nil {
		if tracks, err := s.repo.List(ctx); err == nil {
			for _, t := range tracks {
				if t.Path != "" {
					_ = s.media.Remove(ctx, t.Path)
				}
			}
		}
	}
	return s.repo.DeleteAll(ctx)
}
