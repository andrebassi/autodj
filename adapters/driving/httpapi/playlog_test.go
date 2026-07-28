package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"autodj/domain/library"
	"autodj/domain/playlog"
	"autodj/ports/inbound"
)

// fakePlayLog is a configurable PlayLogService stub. historyOut/historyErr drive
// History; topOut/topErr drive Top. gotFrom/gotTo/gotPeriod/gotN record the last
// call so the handler's query parsing can be asserted.
type fakePlayLog struct {
	historyOut []playlog.Entry
	historyErr error
	topOut     []playlog.TopItem
	topErr     error

	gotFrom, gotTo time.Time
	gotPeriod      string
	gotN           int
}

func (f *fakePlayLog) Record(context.Context, library.Track, time.Time) error { return nil }

func (f *fakePlayLog) History(_ context.Context, from, to time.Time) ([]playlog.Entry, error) {
	f.gotFrom, f.gotTo = from, to
	return f.historyOut, f.historyErr
}

func (f *fakePlayLog) Top(_ context.Context, period string, n int) ([]playlog.TopItem, error) {
	f.gotPeriod, f.gotN = period, n
	return f.topOut, f.topErr
}

func withPlayLog(pl inbound.PlayLogService) http.Handler {
	return New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil, WithPlayLog(pl))
}

func TestGetPlayLogOK(t *testing.T) {
	at := time.Date(2026, 7, 24, 10, 30, 0, 0, time.UTC)
	e, _ := playlog.NewEntry(
		library.Track{ID: "t1", Title: "Aurora", Artist: "Nébula", Source: "local", Duration: 3 * time.Minute}, at)
	pl := &fakePlayLog{historyOut: []playlog.Entry{e}}
	h := withPlayLog(pl)
	rec := do(t, h, http.MethodGet,
		"/api/v1/playlog?from=2026-07-24T00:00:00Z&to=2026-07-25T00:00:00Z", "")
	if rec.Code != 200 {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body)
	}
	if !pl.gotFrom.Equal(time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("from parsed wrong: %v", pl.gotFrom)
	}
	var got []playLogEntryDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].TrackID != "t1" || !got[0].Broadcasted || !got[0].ECADRelevant {
		t.Fatalf("dto=%+v", got)
	}
}

func TestGetPlayLogMissingParams(t *testing.T) {
	pl := &fakePlayLog{}
	h := withPlayLog(pl)
	for _, path := range []string{
		"/api/v1/playlog",
		"/api/v1/playlog?from=2026-07-24T00:00:00Z",
		"/api/v1/playlog?from=2026-07-24T00:00:00Z&to=nonsense",
	} {
		if rec := do(t, h, http.MethodGet, path, ""); rec.Code != 400 {
			t.Fatalf("%s should 400, got %d", path, rec.Code)
		}
	}
}

func TestGetPlayLogRepoError(t *testing.T) {
	pl := &fakePlayLog{historyErr: context.DeadlineExceeded}
	h := withPlayLog(pl)
	rec := do(t, h, http.MethodGet,
		"/api/v1/playlog?from=2026-07-24T00:00:00Z&to=2026-07-25T00:00:00Z", "")
	if rec.Code != 500 {
		t.Fatalf("repo error should 500, got %d", rec.Code)
	}
}

func TestGetPlayLogTopOK(t *testing.T) {
	pl := &fakePlayLog{topOut: []playlog.TopItem{{TrackID: "hot", Title: "Aurora", Artist: "Nébula", Count: 7}}}
	h := withPlayLog(pl)
	rec := do(t, h, http.MethodGet, "/api/v1/playlog/top?period=semana&n=5", "")
	if rec.Code != 200 {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body)
	}
	if pl.gotPeriod != "semana" || pl.gotN != 5 {
		t.Fatalf("period=%q n=%d", pl.gotPeriod, pl.gotN)
	}
	var got []topItemDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].TrackID != "hot" || got[0].Count != 7 {
		t.Fatalf("dto=%+v", got)
	}
}

func TestGetPlayLogTopDefaultsN(t *testing.T) {
	pl := &fakePlayLog{}
	h := withPlayLog(pl)
	// missing n → default 10; out-of-range clamps
	do(t, h, http.MethodGet, "/api/v1/playlog/top?period=dia", "")
	if pl.gotN != 10 {
		t.Fatalf("default n=%d, want 10", pl.gotN)
	}
	do(t, h, http.MethodGet, "/api/v1/playlog/top?period=dia&n=9999", "")
	if pl.gotN != 100 {
		t.Fatalf("clamp high n=%d, want 100", pl.gotN)
	}
	do(t, h, http.MethodGet, "/api/v1/playlog/top?period=dia&n=0", "")
	if pl.gotN != 1 {
		t.Fatalf("clamp low n=%d, want 1", pl.gotN)
	}
}

func TestGetPlayLogTopBadPeriod(t *testing.T) {
	pl := &fakePlayLog{topErr: inbound.ErrBadPeriod}
	h := withPlayLog(pl)
	rec := do(t, h, http.MethodGet, "/api/v1/playlog/top?period=ano", "")
	if rec.Code != 422 {
		t.Fatalf("bad period should 422, got %d", rec.Code)
	}
}

func TestPlayLogRoutesAbsentWithoutOption(t *testing.T) {
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil)
	for _, path := range []string{"/api/v1/playlog?from=x&to=y", "/api/v1/playlog/top?period=dia"} {
		if rec := do(t, h, http.MethodGet, path, ""); rec.Code != 404 {
			t.Fatalf("%s should 404 without WithPlayLog, got %d", path, rec.Code)
		}
	}
}
