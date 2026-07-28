package app

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"autodj/adapters/driven/memory"
	"autodj/adapters/driven/stubengine"
	"autodj/domain/library"
	"autodj/domain/playback"
	"autodj/domain/shared"
	"autodj/ports/inbound"
	"autodj/ports/outbound"
)

type fakeScanner struct {
	paths []string
	err   error
}

func (f fakeScanner) Scan(context.Context, string) ([]string, error) { return f.paths, f.err }

type fakeMeta struct {
	byPath map[string]outbound.Metadata
	fail   map[string]bool
}

func (f fakeMeta) Extract(_ context.Context, p string) (outbound.Metadata, error) {
	if f.fail[p] {
		return outbound.Metadata{}, errors.New("probe fail")
	}
	return f.byPath[p], nil
}

func TestScanFolderHappyPath(t *testing.T) {
	repo := memory.New()
	scan := fakeScanner{paths: []string{"/a.mp3", "/b.flac"}}
	meta := fakeMeta{byPath: map[string]outbound.Metadata{
		"/a.mp3":  {Title: "A", Artist: "X", Duration: 2 * time.Minute},
		"/b.flac": {Title: "B", Artist: "Y", Duration: 3 * time.Minute},
	}}
	svc := NewLibraryService(repo, scan, meta, nil)
	n, err := svc.ScanFolder(context.Background(), "/root")
	if err != nil || n != 2 {
		t.Fatalf("scanned=%d err=%v", n, err)
	}
	got, _ := svc.Tracks(context.Background())
	if len(got) != 2 {
		t.Fatalf("want 2 tracks, got %d", len(got))
	}
	for _, tr := range got {
		if tr.Source != shared.SourceLocal || !tr.Mixable {
			t.Fatalf("scanned tracks must be mixable local: %+v", tr)
		}
	}
	// Re-scan is idempotent (stable id from path → upsert).
	n2, _ := svc.ScanFolder(context.Background(), "/root")
	got2, _ := svc.Tracks(context.Background())
	if n2 != 2 || len(got2) != 2 {
		t.Fatalf("re-scan not idempotent: n=%d total=%d", n2, len(got2))
	}
}

func TestScanFolderSkipsBadFiles(t *testing.T) {
	repo := memory.New()
	scan := fakeScanner{paths: []string{"/ok.mp3", "/bad.mp3", "/zerodur.mp3"}}
	meta := fakeMeta{
		byPath: map[string]outbound.Metadata{
			"/ok.mp3":      {Title: "OK", Duration: time.Minute},
			"/zerodur.mp3": {Title: "Z", Duration: 0}, // invalid → NewTrack fails
		},
		fail: map[string]bool{"/bad.mp3": true}, // probe error → skipped
	}
	svc := NewLibraryService(repo, scan, meta, nil)
	n, err := svc.ScanFolder(context.Background(), "/root")
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("only /ok.mp3 should import, got %d", n)
	}
}

func TestScanFolderUsesPathWhenNoTitle(t *testing.T) {
	repo := memory.New()
	scan := fakeScanner{paths: []string{"/notitle.mp3"}}
	meta := fakeMeta{byPath: map[string]outbound.Metadata{"/notitle.mp3": {Duration: time.Minute}}}
	svc := NewLibraryService(repo, scan, meta, nil)
	if _, err := svc.ScanFolder(context.Background(), "/r"); err != nil {
		t.Fatal(err)
	}
	got, _ := svc.Tracks(context.Background())
	if got[0].Title != "/notitle.mp3" {
		t.Fatalf("title should fall back to path, got %q", got[0].Title)
	}
}

func TestScanFolderScannerError(t *testing.T) {
	svc := NewLibraryService(memory.New(), fakeScanner{err: errors.New("io")}, fakeMeta{}, nil)
	if _, err := svc.ScanFolder(context.Background(), "/r"); err == nil {
		t.Fatal("expected scanner error to propagate")
	}
}

// fakeMedia is an in-memory MediaStore: Save records the content under a fake
// path, Remove deletes from the map. saveErr/removeErr force failures.
type fakeMedia struct {
	saved     map[string][]byte
	saveErr   error
	removeErr error
	removed   []string
}

func newFakeMedia() *fakeMedia { return &fakeMedia{saved: map[string][]byte{}} }

func (f *fakeMedia) Save(_ context.Context, filename string, r io.Reader) (string, error) {
	if f.saveErr != nil {
		return "", f.saveErr
	}
	b, _ := io.ReadAll(r)
	path := "/media/" + filename
	f.saved[path] = b
	return path, nil
}

func (f *fakeMedia) Remove(_ context.Context, path string) error {
	f.removed = append(f.removed, path)
	delete(f.saved, path)
	return f.removeErr
}

func TestUploadHappyPath(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	media := newFakeMedia()
	meta := fakeMeta{byPath: map[string]outbound.Metadata{
		"/media/track.mp3": {Title: "Probed", Artist: "PA", Duration: 2 * time.Minute},
	}}
	svc := NewLibraryService(repo, fakeScanner{}, meta, media)
	tr, err := svc.Upload(ctx, inbound.UploadInput{
		Filename: "track.mp3",
		Content:  strings.NewReader("bytes"),
		Category: "jingle",
	})
	if err != nil {
		t.Fatal(err)
	}
	if tr.Source != shared.SourceLocal || !tr.Mixable {
		t.Fatalf("upload must be local+mixable: %+v", tr)
	}
	if tr.Category != "jingle" || tr.Title != "Probed" || tr.Artist != "PA" {
		t.Fatalf("bad metadata: %+v", tr)
	}
	got, _ := svc.Tracks(ctx)
	if len(got) != 1 {
		t.Fatalf("track should be persisted, got %d", len(got))
	}
}

func TestUploadOperatorOverrides(t *testing.T) {
	ctx := context.Background()
	media := newFakeMedia()
	meta := fakeMeta{byPath: map[string]outbound.Metadata{
		"/media/x.mp3": {Title: "Probed", Artist: "PA", Duration: time.Minute},
	}}
	svc := NewLibraryService(memory.New(), fakeScanner{}, meta, media)
	tr, err := svc.Upload(ctx, inbound.UploadInput{
		Filename: "x.mp3", Content: strings.NewReader("b"),
		Category: "music", Title: "Manual", Artist: "MA", Album: "Alb",
	})
	if err != nil {
		t.Fatal(err)
	}
	if tr.Title != "Manual" || tr.Artist != "MA" || tr.Album != "Alb" {
		t.Fatalf("operator metadata must override probe: %+v", tr)
	}
}

func TestUploadNoMediaStore(t *testing.T) {
	svc := NewLibraryService(memory.New(), fakeScanner{}, fakeMeta{}, nil)
	_, err := svc.Upload(context.Background(), inbound.UploadInput{Filename: "a.mp3", Content: strings.NewReader("b")})
	if err != ErrUploadUnavailable {
		t.Fatalf("want ErrUploadUnavailable, got %v", err)
	}
}

func TestUploadProbeFailCleansUp(t *testing.T) {
	ctx := context.Background()
	media := newFakeMedia()
	meta := fakeMeta{fail: map[string]bool{"/media/bad.mp3": true}}
	svc := NewLibraryService(memory.New(), fakeScanner{}, meta, media)
	_, err := svc.Upload(ctx, inbound.UploadInput{Filename: "bad.mp3", Content: strings.NewReader("b")})
	if err == nil {
		t.Fatal("probe failure should error")
	}
	if len(media.removed) != 1 || len(media.saved) != 0 {
		t.Fatalf("orphan file must be removed on probe fail: removed=%v saved=%v", media.removed, media.saved)
	}
}

func TestUploadBadCategoryCleansUp(t *testing.T) {
	ctx := context.Background()
	media := newFakeMedia()
	meta := fakeMeta{byPath: map[string]outbound.Metadata{"/media/c.mp3": {Title: "T", Duration: time.Minute}}}
	svc := NewLibraryService(memory.New(), fakeScanner{}, meta, media)
	_, err := svc.Upload(ctx, inbound.UploadInput{Filename: "c.mp3", Content: strings.NewReader("b"), Category: "bogus"})
	if err != library.ErrBadCategory {
		t.Fatalf("want ErrBadCategory, got %v", err)
	}
	if len(media.saved) != 0 {
		t.Fatal("bad-category upload must not leak a saved file")
	}
}

func TestUploadSaveError(t *testing.T) {
	media := &fakeMedia{saved: map[string][]byte{}, saveErr: errors.New("disk full")}
	svc := NewLibraryService(memory.New(), fakeScanner{}, fakeMeta{}, media)
	_, err := svc.Upload(context.Background(), inbound.UploadInput{Filename: "a.mp3", Content: strings.NewReader("b")})
	if err == nil {
		t.Fatal("save error must propagate")
	}
}

func TestUpdateTrack(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	media := newFakeMedia()
	tr, _ := library.NewTrack("id1", shared.SourceLocal, "Old", time.Minute)
	tr.Path = "/media/id1.mp3"
	tr.Category = "music"
	_ = repo.Upsert(ctx, tr)
	svc := NewLibraryService(repo, fakeScanner{}, fakeMeta{}, media)

	got, err := svc.UpdateTrack(ctx, "id1", inbound.UpdateInput{
		Title: "New", Artist: "NA", Album: "NAl", Category: "jingle",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "New" || got.Category != "jingle" || got.Artist != "NA" {
		t.Fatalf("update did not apply: %+v", got)
	}
	// Path/source untouched.
	if got.Path != "/media/id1.mp3" || got.Source != shared.SourceLocal {
		t.Fatalf("update must not touch source/path: %+v", got)
	}
}

func TestUpdateTrackNotFound(t *testing.T) {
	svc := NewLibraryService(memory.New(), fakeScanner{}, fakeMeta{}, newFakeMedia())
	_, err := svc.UpdateTrack(context.Background(), "nope", inbound.UpdateInput{Title: "x", Category: "music"})
	if err != library.ErrTrackNotFound {
		t.Fatalf("want ErrTrackNotFound, got %v", err)
	}
}

func TestUpdateTrackBadCategory(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	tr, _ := library.NewTrack("id1", shared.SourceLocal, "T", time.Minute)
	tr.Path = "/m/id1.mp3"
	_ = repo.Upsert(ctx, tr)
	svc := NewLibraryService(repo, fakeScanner{}, fakeMeta{}, newFakeMedia())
	_, err := svc.UpdateTrack(ctx, "id1", inbound.UpdateInput{Title: "T", Category: "bogus"})
	if err != library.ErrBadCategory {
		t.Fatalf("want ErrBadCategory, got %v", err)
	}
}

func TestSetMarkersHappyPath(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	tr, _ := library.NewTrack("id1", shared.SourceLocal, "T", 4*time.Minute)
	tr.Path = "/m/id1.mp3"
	_ = repo.Upsert(ctx, tr)
	svc := NewLibraryService(repo, fakeScanner{}, fakeMeta{}, newFakeMedia())

	got, err := svc.SetMarkers(ctx, "id1", []library.Marker{
		{Kind: library.MarkerHook, PosMs: 60000, Label: "refrão"},
		{Kind: library.MarkerIntro, PosMs: 5000},
	})
	if err != nil {
		t.Fatal(err)
	}
	// Sorted by position: intro (5s) before hook (60s).
	if len(got.Markers) != 2 || got.Markers[0].Kind != library.MarkerIntro {
		t.Fatalf("markers not applied/sorted: %+v", got.Markers)
	}
	// Persisted.
	reload, _ := repo.Get(ctx, "id1")
	if len(reload.Markers) != 2 {
		t.Fatalf("markers not persisted: %+v", reload.Markers)
	}
}

func TestSetMarkersNotFound(t *testing.T) {
	svc := NewLibraryService(memory.New(), fakeScanner{}, fakeMeta{}, newFakeMedia())
	_, err := svc.SetMarkers(context.Background(), "nope", nil)
	if err != library.ErrTrackNotFound {
		t.Fatalf("want ErrTrackNotFound, got %v", err)
	}
}

func TestAudioMixablePath(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	tr, _ := library.NewTrack("id1", shared.SourceLocal, "T", time.Minute)
	tr.Path = "/m/id1.mp3"
	_ = repo.Upsert(ctx, tr)
	svc := NewLibraryService(repo, fakeScanner{}, fakeMeta{}, newFakeMedia())
	got, err := svc.Audio(ctx, "id1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != "/m/id1.mp3" {
		t.Fatalf("path = %q", got.Path)
	}
}

func TestAudioNotFound(t *testing.T) {
	svc := NewLibraryService(memory.New(), fakeScanner{}, fakeMeta{}, newFakeMedia())
	_, err := svc.Audio(context.Background(), "nope")
	if err != library.ErrTrackNotFound {
		t.Fatalf("want ErrTrackNotFound, got %v", err)
	}
}

func TestAudioRejectsSpotify(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	tr, _ := library.NewTrack("sp", shared.SourceSpotify, "Sp", time.Minute)
	tr.SpotifyURI = "spotify:track:z"
	_ = repo.Upsert(ctx, tr)
	svc := NewLibraryService(repo, fakeScanner{}, fakeMeta{}, newFakeMedia())
	_, err := svc.Audio(ctx, "sp")
	if err != library.ErrTrackNotMixable {
		t.Fatalf("want ErrTrackNotMixable, got %v", err)
	}
}

// pathlessRepo returns a mixable/PCM track with no file path — a state the
// validating memory repo would reject on Upsert — so Audio() can exercise the
// ErrNoAudioPath guard directly.
type pathlessRepo struct{ outbound.TrackRepo }

func (pathlessRepo) Get(context.Context, string) (*library.Track, error) {
	return &library.Track{ID: "id1", Source: shared.SourceLocal, Mixable: true, PCMAvailable: true}, nil
}

func TestAudioNoPath(t *testing.T) {
	svc := NewLibraryService(pathlessRepo{}, fakeScanner{}, fakeMeta{}, newFakeMedia())
	_, err := svc.Audio(context.Background(), "id1")
	if err != library.ErrNoAudioPath {
		t.Fatalf("want ErrNoAudioPath, got %v", err)
	}
}

func TestSetMarkersRejectsNonPCM(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	tr, _ := library.NewTrack("sp", shared.SourceSpotify, "Sp", time.Minute)
	tr.SpotifyURI = "spotify:track:z"
	_ = repo.Upsert(ctx, tr)
	svc := NewLibraryService(repo, fakeScanner{}, fakeMeta{}, newFakeMedia())
	_, err := svc.SetMarkers(ctx, "sp", []library.Marker{{Kind: library.MarkerIntro, PosMs: 1000}})
	if err != library.ErrMarkersNotPCM {
		t.Fatalf("want ErrMarkersNotPCM, got %v", err)
	}
}

func TestSetMarkersRejectsBadKind(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	tr, _ := library.NewTrack("id1", shared.SourceLocal, "T", time.Minute)
	tr.Path = "/m/id1.mp3"
	_ = repo.Upsert(ctx, tr)
	svc := NewLibraryService(repo, fakeScanner{}, fakeMeta{}, newFakeMedia())
	_, err := svc.SetMarkers(ctx, "id1", []library.Marker{{Kind: "bogus", PosMs: 0}})
	if err != library.ErrBadMarkerKind {
		t.Fatalf("want ErrBadMarkerKind, got %v", err)
	}
}

func TestDeleteTrack(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	media := newFakeMedia()
	tr, _ := library.NewTrack("id1", shared.SourceLocal, "T", time.Minute)
	tr.Path = "/media/id1.mp3"
	_ = repo.Upsert(ctx, tr)
	svc := NewLibraryService(repo, fakeScanner{}, fakeMeta{}, media)

	if err := svc.DeleteTrack(ctx, "id1"); err != nil {
		t.Fatal(err)
	}
	got, _ := svc.Tracks(ctx)
	if len(got) != 0 {
		t.Fatal("track should be gone")
	}
	if len(media.removed) != 1 || media.removed[0] != "/media/id1.mp3" {
		t.Fatalf("media file should be removed: %v", media.removed)
	}
}

func TestDeleteTrackNotFound(t *testing.T) {
	svc := NewLibraryService(memory.New(), fakeScanner{}, fakeMeta{}, newFakeMedia())
	if err := svc.DeleteTrack(context.Background(), "nope"); err != library.ErrTrackNotFound {
		t.Fatalf("want ErrTrackNotFound, got %v", err)
	}
}

func TestDeleteTrackNoMediaOK(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	tr, _ := library.NewTrack("id1", shared.SourceLocal, "T", time.Minute)
	tr.Path = "/media/id1.mp3"
	_ = repo.Upsert(ctx, tr)
	// media nil: delete still succeeds (repo is source of truth).
	svc := NewLibraryService(repo, fakeScanner{}, fakeMeta{}, nil)
	if err := svc.DeleteTrack(ctx, "id1"); err != nil {
		t.Fatal(err)
	}
}

// fakeWaveform is a stub WaveformGenerator: returns fixed peaks (or an error).
type fakeWaveform struct {
	peaks []float64
	err   error
}

func (f fakeWaveform) Peaks(context.Context, *library.Track, int) ([]float64, error) {
	return f.peaks, f.err
}

func TestWaveformHappyPath(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	tr, _ := library.NewTrack("id1", shared.SourceLocal, "T", 5*time.Minute)
	tr.Path = "/m/id1.mp3"
	_ = repo.Upsert(ctx, tr)
	svc := NewLibraryService(repo, fakeScanner{}, fakeMeta{}, nil)
	svc.SetWaveformGenerator(fakeWaveform{peaks: []float64{0.1, 0.9, 0.4}})

	wf, err := svc.Waveform(ctx, "id1", 3)
	if err != nil {
		t.Fatal(err)
	}
	if wf.DurationMs != (5 * time.Minute).Milliseconds() || len(wf.Peaks) != 3 || wf.Peaks[1] != 0.9 {
		t.Fatalf("bad waveform: %+v", wf)
	}
}

func TestWaveformNoGenerator(t *testing.T) {
	svc := NewLibraryService(memory.New(), fakeScanner{}, fakeMeta{}, nil)
	_, err := svc.Waveform(context.Background(), "id1", 0)
	if err != ErrWaveformUnavailable {
		t.Fatalf("want ErrWaveformUnavailable, got %v", err)
	}
}

func TestWaveformNotFound(t *testing.T) {
	svc := NewLibraryService(memory.New(), fakeScanner{}, fakeMeta{}, nil)
	svc.SetWaveformGenerator(fakeWaveform{peaks: []float64{0.5}})
	_, err := svc.Waveform(context.Background(), "nope", 0)
	if err != library.ErrTrackNotFound {
		t.Fatalf("want ErrTrackNotFound, got %v", err)
	}
}

func TestWaveformRejectsNonPCM(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	tr, _ := library.NewTrack("sp", shared.SourceSpotify, "Sp", time.Minute)
	tr.SpotifyURI = "spotify:track:z"
	_ = repo.Upsert(ctx, tr)
	svc := NewLibraryService(repo, fakeScanner{}, fakeMeta{}, nil)
	svc.SetWaveformGenerator(fakeWaveform{peaks: []float64{0.5}})
	_, err := svc.Waveform(ctx, "sp", 0)
	if err != library.ErrWaveformNotPCM {
		t.Fatalf("want ErrWaveformNotPCM, got %v", err)
	}
}

func TestWaveformGeneratorError(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	tr, _ := library.NewTrack("id1", shared.SourceLocal, "T", time.Minute)
	tr.Path = "/m/id1.mp3"
	_ = repo.Upsert(ctx, tr)
	svc := NewLibraryService(repo, fakeScanner{}, fakeMeta{}, nil)
	svc.SetWaveformGenerator(fakeWaveform{err: errors.New("peaks io")})
	if _, err := svc.Waveform(ctx, "id1", 0); err == nil {
		t.Fatal("generator error must propagate")
	}
}

func TestCategoriesService(t *testing.T) {
	svc := NewLibraryService(memory.New(), fakeScanner{}, fakeMeta{}, nil)
	if len(svc.Categories()) != len(library.Categories()) {
		t.Fatal("service categories must mirror domain")
	}
}

func TestPlaybackLoadAndNext(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	// seed a local (mixable) + a spotify (not mixable) track.
	loc, _ := library.NewTrack("t1", shared.SourceLocal, "Local", time.Minute)
	loc.Path = "/l.mp3"
	_ = repo.Upsert(ctx, loc)
	sp := &library.Track{ID: "t2", Source: shared.SourceSpotify, Title: "Sp", SpotifyURI: "spotify:x", Duration: time.Minute}
	_ = repo.Upsert(ctx, sp)

	eng := stubengine.New()
	svc := NewPlaybackService(repo, eng)
	n, err := svc.LoadFromLibrary(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("only the local track is mixable; queued=%d", n)
	}
	np, q, err := svc.NowPlaying(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if np.Title != "Local" {
		t.Fatalf("now-playing should be Local, got %q", np.Title)
	}
	_ = q
	if err := svc.Next(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestPlaybackEnqueue(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	for _, id := range []string{"a", "b"} {
		tr, _ := library.NewTrack(id, shared.SourceLocal, id, time.Minute)
		tr.Path = "/" + id + ".mp3"
		_ = repo.Upsert(ctx, tr)
	}
	sp := &library.Track{ID: "s", Source: shared.SourceSpotify, Title: "Sp", SpotifyURI: "spotify:x", Duration: time.Minute}
	_ = repo.Upsert(ctx, sp)

	eng := stubengine.New()
	svc := NewPlaybackService(repo, eng)
	// engineHead asserts the engine's now-playing matches the mirror head — the
	// invariant that keeps "próxima"/"tocar agora" playing what the UI shows.
	engineHead := func(want string) {
		t.Helper()
		np, _ := eng.NowPlaying(ctx)
		if np.TrackID != want {
			t.Fatalf("engine now-playing = %q, want %q", np.TrackID, want)
		}
	}

	// append (pos<0): a becomes the sole now-playing (engine head).
	if err := svc.Enqueue(ctx, "a", -1, false, ""); err != nil {
		t.Fatalf("append a: %v", err)
	}
	engineHead("a")
	// pos 0 without playNow ("inserir após atual") must NOT displace the on-air
	// head — b lands right after a (próxima), never before it.
	if err := svc.Enqueue(ctx, "b", 0, false, ""); err != nil {
		t.Fatalf("após atual b: %v", err)
	}
	q := svc.Queue(ctx)
	if len(q) != 2 || q[0].Playable.TrackID != "a" || q[1].Playable.TrackID != "b" {
		t.Fatalf("mirror order wrong: %+v", q)
	}
	engineHead("a")

	// playNow ("tocar agora") cuts the on-air head and plays the new item now, on
	// BOTH the mirror and the engine (the desync bug: engine kept the old next).
	if err := svc.Enqueue(ctx, "b", 0, true, ""); err != nil {
		t.Fatalf("playNow: %v", err)
	}
	if svc.Queue(ctx)[0].Playable.TrackID != "b" {
		t.Fatal("playNow item must be the head")
	}
	engineHead("b")

	// spotify IS schedulable (plays out-of-band via the browser SDK, hard_cut).
	// The stub engine accepts it; it lands in the queue like any other item.
	if err := svc.Enqueue(ctx, "s", -1, false, "Minha Playlist"); err != nil {
		t.Fatalf("spotify enqueue = %v, want nil", err)
	}
	last := svc.Queue(ctx)
	if last[len(last)-1].Playable.TrackID != "s" {
		t.Fatalf("spotify item must be queued at the back: %+v", last)
	}
	// origin flows through to the queued item (provenance column).
	if last[len(last)-1].Playable.Origin != "Minha Playlist" {
		t.Fatalf("spotify origin = %q, want %q", last[len(last)-1].Playable.Origin, "Minha Playlist")
	}
	if last[len(last)-1].Transition != shared.TransitionHardCut {
		t.Fatalf("spotify transition = %v, want hard_cut", last[len(last)-1].Transition)
	}

	// unknown track propagates repo error
	if err := svc.Enqueue(ctx, "nope", -1, false, ""); err == nil {
		t.Fatal("unknown track must error")
	}
}

// TestPlaybackEnqueueEngineSync locks the mirror↔engine ordering: an item cued as
// the próxima (pos 1) over a populated queue must be what the engine plays next —
// not the pre-existing next (the "toca a próxima da fila, não a que pedi" bug).
func TestPlaybackEnqueueEngineSync(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	for _, id := range []string{"a", "b", "c", "x"} {
		tr, _ := library.NewTrack(id, shared.SourceLocal, id, time.Minute)
		tr.Path = "/" + id + ".mp3"
		_ = repo.Upsert(ctx, tr)
	}
	eng := stubengine.New()
	svc := NewPlaybackService(repo, eng)
	for _, id := range []string{"a", "b", "c"} { // mirror: [a(head) b c]
		if err := svc.Enqueue(ctx, id, -1, false, ""); err != nil {
			t.Fatalf("append %s: %v", id, err)
		}
	}
	// cue x as the próxima (mirror index 1) — never displacing the head a.
	if err := svc.Enqueue(ctx, "x", 1, false, ""); err != nil {
		t.Fatalf("cue x: %v", err)
	}
	if q := svc.Queue(ctx); q[0].Playable.TrackID != "a" || q[1].Playable.TrackID != "x" {
		t.Fatalf("mirror after cue = %+v, want head a, próxima x", q)
	}
	// The engine must play a → x → b → c, in the exact mirror order.
	for _, want := range []string{"a", "x", "b", "c"} {
		np, _ := eng.NowPlaying(ctx)
		if np.TrackID != want {
			t.Fatalf("engine head = %q, want %q", np.TrackID, want)
		}
		_ = eng.Skip(ctx)
	}
}

func TestPlaybackDequeue(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	for _, id := range []string{"a", "b", "c"} {
		tr, _ := library.NewTrack(id, shared.SourceLocal, id, time.Minute)
		tr.Path = "/" + id + ".mp3"
		_ = repo.Upsert(ctx, tr)
	}
	svc := NewPlaybackService(repo, stubengine.New())
	for _, id := range []string{"a", "b", "c"} {
		if err := svc.Enqueue(ctx, id, -1, false, ""); err != nil {
			t.Fatalf("enqueue %s: %v", id, err)
		}
	}
	// pos 0 is the now-playing head — refused (use Next to skip it).
	if err := svc.Dequeue(ctx, 0); err != playback.ErrEmptyQueue {
		t.Fatalf("dequeue 0 = %v, want ErrEmptyQueue", err)
	}
	// out-of-range refused too.
	if err := svc.Dequeue(ctx, 9); err != playback.ErrEmptyQueue {
		t.Fatalf("dequeue 9 = %v, want ErrEmptyQueue", err)
	}
	// remove the middle pending item (b, index 1) → [a, c] remain.
	if err := svc.Dequeue(ctx, 1); err != nil {
		t.Fatalf("dequeue 1: %v", err)
	}
	q := svc.Queue(ctx)
	if len(q) != 2 || q[0].Playable.TrackID != "a" || q[1].Playable.TrackID != "c" {
		t.Fatalf("after dequeue got %+v, want [a c]", q)
	}
}

func TestPlaybackMove(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	for _, id := range []string{"a", "b", "c", "d"} {
		tr, _ := library.NewTrack(id, shared.SourceLocal, id, time.Minute)
		tr.Path = "/" + id + ".mp3"
		_ = repo.Upsert(ctx, tr)
	}
	svc := NewPlaybackService(repo, stubengine.New())
	for _, id := range []string{"a", "b", "c", "d"} {
		if err := svc.Enqueue(ctx, id, -1, false, ""); err != nil {
			t.Fatalf("enqueue %s: %v", id, err)
		}
	}
	// index 0 (now-playing a) is off-limits on either end.
	if err := svc.Move(ctx, 0, 2); err != playback.ErrEmptyQueue {
		t.Fatalf("move 0→2 = %v, want ErrEmptyQueue", err)
	}
	if err := svc.Move(ctx, 2, 0); err != playback.ErrEmptyQueue {
		t.Fatalf("move 2→0 = %v, want ErrEmptyQueue", err)
	}
	// mirror indices: 0=a(cur) 1=b 2=c 3=d. Move d (3) up to just after a (1)
	// ⇒ a, d, b, c.
	if err := svc.Move(ctx, 3, 1); err != nil {
		t.Fatalf("move 3→1: %v", err)
	}
	q := svc.Queue(ctx)
	got := ""
	for _, it := range q {
		got += it.Playable.TrackID
	}
	if got != "adbc" {
		t.Fatalf("after move got %s, want adbc", got)
	}
}

func TestPlaybackClear(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	for _, id := range []string{"a", "b", "c"} {
		tr, _ := library.NewTrack(id, shared.SourceLocal, id, time.Minute)
		tr.Path = "/" + id + ".mp3"
		_ = repo.Upsert(ctx, tr)
	}
	svc := NewPlaybackService(repo, stubengine.New())
	for _, id := range []string{"a", "b", "c"} {
		if err := svc.Enqueue(ctx, id, -1, false, ""); err != nil {
			t.Fatalf("enqueue %s: %v", id, err)
		}
	}
	if err := svc.Clear(ctx); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if q := svc.Queue(ctx); len(q) != 0 {
		t.Fatalf("after clear got %+v, want empty", q)
	}
	// clearing an already-empty playout is a no-op, not an error.
	if err := svc.Clear(ctx); err != nil {
		t.Fatalf("clear empty: %v", err)
	}
}

// Artwork keeps a YouTube track alive even with no stored ArtworkURL and no local
// Path: its YouTubeURI lets the HTTP layer derive the public ytimg thumbnail.
func TestArtworkYouTubeNoStoredURL(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	tr, _ := library.NewTrack("yt", shared.SourceYouTube, "Song", time.Minute)
	tr.YouTubeURI = "youtube:video:2bS-rippO6I"
	_ = repo.Upsert(ctx, tr)
	svc := NewLibraryService(repo, fakeScanner{}, fakeMeta{}, newFakeMedia())

	got, err := svc.Artwork(ctx, "yt")
	if err != nil {
		t.Fatalf("want track, got err %v", err)
	}
	if got.YouTubeURI != "youtube:video:2bS-rippO6I" {
		t.Fatalf("lost YouTubeURI: %q", got.YouTubeURI)
	}
}

// A track with no cover source at all → ErrNoArtwork (404 → placeholder).
func TestArtworkNoSource(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	tr, _ := library.NewTrack("x", shared.SourceSpotify, "Song", time.Minute)
	tr.SpotifyURI = "spotify:track:abc"
	tr.Path = ""
	tr.ArtworkURL = ""
	_ = repo.Upsert(ctx, tr)
	svc := NewLibraryService(repo, fakeScanner{}, fakeMeta{}, newFakeMedia())

	if _, err := svc.Artwork(ctx, "x"); !errors.Is(err, library.ErrNoArtwork) {
		t.Fatalf("want ErrNoArtwork, got %v", err)
	}
}
