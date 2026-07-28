package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"autodj/domain/content"
	"autodj/domain/library"
	"autodj/domain/mixing"
	"autodj/domain/playback"
	"autodj/domain/schedule"
	"autodj/domain/shared"
	"autodj/ports/inbound"
)

type fakeLib struct {
	scanned  int
	scanErr  error
	tracks   []*library.Track
	uploaded *library.Track
	upErr    error
	updated  *library.Track
	updErr   error
	delErr   error
	clearErr error
	marked   *library.Track
	markErr  error
	wf       inbound.WaveformData
	wfErr    error
	audioT     *library.Track
	audioErr   error
	artworkT   *library.Track
	artworkErr error

	folders    []library.FolderCount
	foldersErr error
	crFoldErr  error
	delFoldErr error
	setFoldT   *library.Track
	setFoldErr error
}

func (f fakeLib) ScanFolder(context.Context, string) (int, error) { return f.scanned, f.scanErr }
func (f fakeLib) Tracks(context.Context) ([]*library.Track, error) {
	return f.tracks, nil
}
func (f fakeLib) Categories() []library.Category { return library.Categories() }
func (f fakeLib) Upload(context.Context, inbound.UploadInput) (*library.Track, error) {
	return f.uploaded, f.upErr
}
func (f fakeLib) UpdateTrack(context.Context, string, inbound.UpdateInput) (*library.Track, error) {
	return f.updated, f.updErr
}
func (f fakeLib) DeleteTrack(context.Context, string) error { return f.delErr }
func (f fakeLib) ClearLibrary(context.Context) error         { return f.clearErr }
func (f fakeLib) SetMarkers(context.Context, string, []library.Marker) (*library.Track, error) {
	return f.marked, f.markErr
}
func (f fakeLib) Waveform(context.Context, string, int) (inbound.WaveformData, error) {
	return f.wf, f.wfErr
}
func (f fakeLib) Audio(context.Context, string) (*library.Track, error) {
	return f.audioT, f.audioErr
}
func (f fakeLib) Artwork(context.Context, string) (*library.Track, error) {
	return f.artworkT, f.artworkErr
}
func (f fakeLib) Folders(context.Context) ([]library.FolderCount, error) {
	return f.folders, f.foldersErr
}
func (f fakeLib) CreateFolder(context.Context, string) error { return f.crFoldErr }
func (f fakeLib) DeleteFolder(context.Context, string) error { return f.delFoldErr }
func (f fakeLib) SetTrackFolders(context.Context, string, []string) (*library.Track, error) {
	return f.setFoldT, f.setFoldErr
}

type fakePlay struct {
	queued int
	np     playback.Playable
	q      []playback.QueueItem
	err    error
}

func (f fakePlay) LoadFromLibrary(context.Context) (int, error) { return f.queued, f.err }
func (f fakePlay) Next(context.Context) error                   { return f.err }
func (f fakePlay) Clear(context.Context) error                  { return f.err }
func (f fakePlay) Enqueue(context.Context, string, int, bool, string) error { return f.err }
func (f fakePlay) Dequeue(context.Context, int) error                       { return f.err }
func (f fakePlay) Move(context.Context, int, int) error                     { return f.err }
func (f fakePlay) NowPlaying(context.Context) (playback.Playable, []playback.QueueItem, error) {
	return f.np, f.q, f.err
}
func (f fakePlay) Queue(context.Context) []playback.QueueItem { return f.q }

type fakeContent struct{}

func (fakeContent) Operational(context.Context) content.Operational {
	return content.OperationalCatalog()
}
func (fakeContent) WebTabs(context.Context) []content.WebTab { return content.WebTabs() }
func (fakeContent) Program(context.Context) content.Program  { return content.ProgramPreview() }
func (fakeContent) GradeLoad(context.Context) content.GradeLoad {
	return content.GradeLoadPreview()
}

// fakeMix is a configurable MixService stub. Zero value serves mixing.Default()
// and accepts any save.
type fakeMix struct {
	get     mixing.Settings
	getSet  bool
	getErr  error
	saveErr error
}

func (f fakeMix) Get(context.Context) (mixing.Settings, error) {
	if f.getErr != nil {
		return mixing.Settings{}, f.getErr
	}
	if f.getSet {
		return f.get, nil
	}
	return mixing.Default(), nil
}

func (f fakeMix) Save(_ context.Context, s mixing.Settings) (mixing.Settings, error) {
	if f.saveErr != nil {
		return mixing.Settings{}, f.saveErr
	}
	return s, nil
}

type fakeSched struct {
	clocks    []schedule.Clock
	saved     schedule.Clock
	clocksErr error
	saveErr   error
	delErr    error
	grid      schedule.Grid
	gridErr   error
	saveGrid  schedule.Grid
	gridSvErr error
	genTracks []library.Track
	genErr    error
	events    []schedule.ScheduledEvent
	eventsErr error
	savedEv   schedule.ScheduledEvent
	saveEvErr error
	delEvErr  error
	injTracks []library.Track
	injErr    error
}

func (f fakeSched) Clocks(context.Context) ([]schedule.Clock, error) {
	return f.clocks, f.clocksErr
}

func (f fakeSched) SaveClock(_ context.Context, c schedule.Clock) (schedule.Clock, error) {
	if f.saveErr != nil {
		return schedule.Clock{}, f.saveErr
	}
	if f.saved.ID != "" {
		return f.saved, nil
	}
	return c, nil
}

func (f fakeSched) DeleteClock(context.Context, string) error { return f.delErr }

func (f fakeSched) Grid(context.Context) (schedule.Grid, error) {
	return f.grid, f.gridErr
}

func (f fakeSched) SaveGrid(_ context.Context, g schedule.Grid) (schedule.Grid, error) {
	if f.gridSvErr != nil {
		return schedule.Grid{}, f.gridSvErr
	}
	if len(f.saveGrid.Cells) != 0 {
		return f.saveGrid, nil
	}
	return g, nil
}

func (f fakeSched) GeneratePlaylist(context.Context, string) ([]library.Track, error) {
	return f.genTracks, f.genErr
}

func (f fakeSched) Events(context.Context) ([]schedule.ScheduledEvent, error) {
	return f.events, f.eventsErr
}

func (f fakeSched) SaveEvent(_ context.Context, e schedule.ScheduledEvent) (schedule.ScheduledEvent, error) {
	if f.saveEvErr != nil {
		return schedule.ScheduledEvent{}, f.saveEvErr
	}
	if f.savedEv.ID != "" {
		return f.savedEv, nil
	}
	return e, nil
}

func (f fakeSched) DeleteEvent(context.Context, string) error { return f.delEvErr }

func (f fakeSched) NextInjections(context.Context, time.Time) ([]library.Track, error) {
	return f.injTracks, f.injErr
}

func do(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// doReq runs a prebuilt request (for headers like Range) through the handler.
func doReq(t *testing.T, h http.Handler, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestHealth(t *testing.T) {
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil)
	rec := do(t, h, http.MethodGet, "/api/v1/health", "")
	if rec.Code != 200 {
		t.Fatal(rec.Code)
	}
	var m map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &m)
	if m["status"] != "ok" || m["version"] != Version {
		t.Fatalf("bad health: %v", m)
	}
}

// Version é var (injetada por ldflags no Docker) justamente pra que o health reporte a
// tag do release — é dela que o front deduz "servidor trocou embaixo da aba". Se alguém
// voltar a const, o build quebra aqui antes de a checagem virar um no-op silencioso.
func TestHealth_reportaVersaoInjetada(t *testing.T) {
	orig := Version
	t.Cleanup(func() { Version = orig })
	Version = "v9.9.9"

	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil)
	rec := do(t, h, http.MethodGet, "/api/v1/health", "")
	var m map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &m)
	if m["version"] != "v9.9.9" {
		t.Fatalf("version = %q, want a injetada v9.9.9", m["version"])
	}
}

// Build local (sem ldflags) tem que sair como "dev": é esse valor que faz o front
// DESLIGAR a checagem, porque fora do Docker bundle e binário são compilados em
// momentos diferentes e divergiriam à toa.
func TestHealth_defaultEhDev(t *testing.T) {
	if Version != "dev" {
		t.Fatalf("Version default = %q, want \"dev\" (sem ldflags)", Version)
	}
}

func TestScanEndpoint(t *testing.T) {
	h := New(fakeLib{scanned: 5}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil)
	rec := do(t, h, http.MethodPost, "/api/v1/library/scan", `{"root":"/music"}`)
	if rec.Code != 200 {
		t.Fatal(rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"scanned":5`) {
		t.Fatalf("body: %s", rec.Body.String())
	}
}

func TestScanEndpointBadBody(t *testing.T) {
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil)
	if rec := do(t, h, http.MethodPost, "/api/v1/library/scan", `{}`); rec.Code != 400 {
		t.Fatalf("empty root should be 400, got %d", rec.Code)
	}
	if rec := do(t, h, http.MethodPost, "/api/v1/library/scan", `not json`); rec.Code != 400 {
		t.Fatalf("bad json should be 400, got %d", rec.Code)
	}
}

func TestScanEndpointError(t *testing.T) {
	h := New(fakeLib{scanErr: errors.New("boom")}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil)
	if rec := do(t, h, http.MethodPost, "/api/v1/library/scan", `{"root":"/x"}`); rec.Code != 500 {
		t.Fatalf("want 500, got %d", rec.Code)
	}
}

func TestTracksEndpoint(t *testing.T) {
	tr, _ := library.NewTrack("1", shared.SourceLocal, "Song", time.Minute)
	tr.Artist = "A"
	h := New(fakeLib{tracks: []*library.Track{tr}}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil)
	rec := do(t, h, http.MethodGet, "/api/v1/library/tracks", "")
	if rec.Code != 200 {
		t.Fatal(rec.Code)
	}
	var got []trackDTO
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if len(got) != 1 || got[0].Title != "Song" || !got[0].Mixable {
		t.Fatalf("bad track dto: %+v", got)
	}
}

func TestLoadNextEndpoints(t *testing.T) {
	h := New(fakeLib{}, fakePlay{queued: 3}, fakeContent{}, fakeMix{}, fakeSched{}, nil)
	if rec := do(t, h, http.MethodPost, "/api/v1/playback/load", ""); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"queued":3`) {
		t.Fatalf("load: %d %s", rec.Code, rec.Body.String())
	}
	if rec := do(t, h, http.MethodPost, "/api/v1/playback/next", ""); rec.Code != 200 {
		t.Fatalf("next: %d", rec.Code)
	}
	if rec := do(t, h, http.MethodPost, "/api/v1/playback/clear", ""); rec.Code != 200 {
		t.Fatalf("clear: %d", rec.Code)
	}
}

func TestLoadNextErrors(t *testing.T) {
	h := New(fakeLib{}, fakePlay{err: errors.New("x")}, fakeContent{}, fakeMix{}, fakeSched{}, nil)
	if rec := do(t, h, http.MethodPost, "/api/v1/playback/load", ""); rec.Code != 500 {
		t.Fatal("load err")
	}
	if rec := do(t, h, http.MethodPost, "/api/v1/playback/next", ""); rec.Code != 500 {
		t.Fatal("next err")
	}
	if rec := do(t, h, http.MethodPost, "/api/v1/playback/clear", ""); rec.Code != 500 {
		t.Fatal("clear err")
	}
}

func TestEnqueueEndpoint(t *testing.T) {
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil)
	// happy path
	if rec := do(t, h, http.MethodPost, "/api/v1/playback/enqueue", `{"track_id":"t1","pos":0,"play_now":true}`); rec.Code != 200 {
		t.Fatalf("enqueue: %d %s", rec.Code, rec.Body.String())
	}
	// missing track_id → 400
	if rec := do(t, h, http.MethodPost, "/api/v1/playback/enqueue", `{"pos":0}`); rec.Code != 400 {
		t.Fatalf("missing track_id want 400, got %d", rec.Code)
	}
	// bad json → 400
	if rec := do(t, h, http.MethodPost, "/api/v1/playback/enqueue", `{`); rec.Code != 400 {
		t.Fatalf("bad json want 400, got %d", rec.Code)
	}
	// service error (e.g. not mixable) → 422
	he := New(fakeLib{}, fakePlay{err: errors.New("not mixable")}, fakeContent{}, fakeMix{}, fakeSched{}, nil)
	if rec := do(t, he, http.MethodPost, "/api/v1/playback/enqueue", `{"track_id":"s"}`); rec.Code != 422 {
		t.Fatalf("service err want 422, got %d", rec.Code)
	}
}

func TestDequeueEndpoint(t *testing.T) {
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil)
	// happy path
	if rec := do(t, h, http.MethodPost, "/api/v1/playback/dequeue", `{"pos":2}`); rec.Code != 200 {
		t.Fatalf("dequeue: %d %s", rec.Code, rec.Body.String())
	}
	// bad json → 400
	if rec := do(t, h, http.MethodPost, "/api/v1/playback/dequeue", `{`); rec.Code != 400 {
		t.Fatalf("bad json want 400, got %d", rec.Code)
	}
	// service error (e.g. pos 0 / out of range) → 422
	he := New(fakeLib{}, fakePlay{err: errors.New("empty queue")}, fakeContent{}, fakeMix{}, fakeSched{}, nil)
	if rec := do(t, he, http.MethodPost, "/api/v1/playback/dequeue", `{"pos":1}`); rec.Code != 422 {
		t.Fatalf("service err want 422, got %d", rec.Code)
	}
}

func TestNowPlayingEndpoint(t *testing.T) {
	np := playback.Playable{TrackID: "1", Title: "Live", Source: shared.SourceLocal, Duration: time.Minute}
	q := []playback.QueueItem{{
		Playable:   playback.Playable{TrackID: "2", Title: "Next", Source: shared.SourceLocal},
		Transition: shared.TransitionCrossfade,
	}}
	h := New(fakeLib{}, fakePlay{np: np, q: q}, fakeContent{}, fakeMix{}, fakeSched{}, nil)
	rec := do(t, h, http.MethodGet, "/api/v1/playback/nowplaying", "")
	var m nowPlayingMsg
	_ = json.Unmarshal(rec.Body.Bytes(), &m)
	if m.NowPlaying.Title != "Live" || len(m.Queue) != 1 || m.Queue[0].Transition != "crossfade" {
		t.Fatalf("bad nowplaying: %+v", m)
	}
}

func TestNowPlayingError(t *testing.T) {
	h := New(fakeLib{}, fakePlay{err: errors.New("x")}, fakeContent{}, fakeMix{}, fakeSched{}, nil)
	if rec := do(t, h, http.MethodGet, "/api/v1/playback/nowplaying", ""); rec.Code != 500 {
		t.Fatal("want 500")
	}
}

// O preflight segue respondendo 204 mesmo sem Origin liberada — o que muda é que
// NÃO acompanha permissão. Este teste afirmava `*`; a permissão por origem
// conhecida está coberta em cors_test.go.
func TestCORSPreflight(t *testing.T) {
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil)
	rec := do(t, h, http.MethodOptions, "/api/v1/health", "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("preflight should be 204, got %d", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got == "*" {
		t.Fatal("curinga não pode voltar: qualquer site falaria com o AutoDJ local")
	}
}

func TestUIEndpoints(t *testing.T) {
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil)

	rec := do(t, h, http.MethodGet, "/api/v1/ui/operational", "")
	if rec.Code != 200 {
		t.Fatalf("operational: %d", rec.Code)
	}
	var op content.Operational
	_ = json.Unmarshal(rec.Body.Bytes(), &op)
	if len(op.Categories) == 0 || len(op.Cartwall) == 0 || op.Mix.NormalizeVol != 89 {
		t.Fatalf("bad operational: %+v", op)
	}

	rec = do(t, h, http.MethodGet, "/api/v1/ui/webtabs", "")
	if rec.Code != 200 {
		t.Fatalf("webtabs: %d", rec.Code)
	}
	var tabs []content.WebTab
	_ = json.Unmarshal(rec.Body.Bytes(), &tabs)
	if len(tabs) == 0 || tabs[0].Name == "" {
		t.Fatalf("bad webtabs: %+v", tabs)
	}

	rec = do(t, h, http.MethodGet, "/api/v1/ui/program", "")
	if rec.Code != 200 {
		t.Fatalf("program: %d", rec.Code)
	}
	var pg content.Program
	_ = json.Unmarshal(rec.Body.Bytes(), &pg)
	if len(pg.Above) == 0 || len(pg.Below) == 0 {
		t.Fatalf("bad program: %+v", pg)
	}

	rec = do(t, h, http.MethodGet, "/api/v1/ui/grade-load", "")
	if rec.Code != 200 {
		t.Fatalf("grade-load: %d", rec.Code)
	}
	var gl content.GradeLoad
	_ = json.Unmarshal(rec.Body.Bytes(), &gl)
	if len(gl.Hourly) != 24 {
		t.Fatalf("bad grade-load: %+v", gl)
	}
}

func TestWSHandlerMounted(t *testing.T) {
	called := false
	wsH := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(200)
	})
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, wsH)
	do(t, h, http.MethodGet, "/ws", "")
	if !called {
		t.Fatal("ws handler should be mounted")
	}
}
