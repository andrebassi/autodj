package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"autodj/domain/reports"
	"autodj/domain/shared"
	"autodj/ports/inbound"
)

// fakeReports is a configurable ReportService stub. out/err drive ECAD, proof/err
// drive BroadcastProof; gotFrom/gotTo record the last call so query parsing can be
// asserted.
type fakeReports struct {
	out            reports.ECADReport
	proof          reports.BroadcastProof
	err            error
	gotFrom, gotTo time.Time
}

func (f *fakeReports) ECAD(_ context.Context, from, to time.Time) (reports.ECADReport, error) {
	f.gotFrom, f.gotTo = from, to
	return f.out, f.err
}

func (f *fakeReports) BroadcastProof(_ context.Context, from, to time.Time) (reports.BroadcastProof, error) {
	f.gotFrom, f.gotTo = from, to
	return f.proof, f.err
}

func withReports(rp inbound.ReportService) http.Handler {
	return New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil, WithReports(rp))
}

func TestGetECADReportOK(t *testing.T) {
	from := time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)
	to := from.Add(24 * time.Hour)
	rp := &fakeReports{out: reports.ECADReport{
		From: from, To: to, GeneratedAt: to.Add(time.Hour),
		Lines:           []reports.ECADLine{{TrackID: "a", Title: "Aurora", Artist: "Nébula", Plays: 3, TotalDurationMs: 540000}},
		TotalPlays:      3,
		TotalDurationMs: 540000,
	}}
	h := withReports(rp)
	rec := do(t, h, http.MethodGet,
		"/api/v1/reports/ecad?from=2026-07-24T00:00:00Z&to=2026-07-25T00:00:00Z", "")
	if rec.Code != 200 {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body)
	}
	if !rp.gotFrom.Equal(from) || !rp.gotTo.Equal(to) {
		t.Fatalf("range parsed wrong: from=%v to=%v", rp.gotFrom, rp.gotTo)
	}
	var got ecadReportDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.TotalPlays != 3 || len(got.Lines) != 1 || got.Lines[0].TrackID != "a" || got.Lines[0].Plays != 3 {
		t.Fatalf("dto=%+v", got)
	}
}

func TestGetECADReportMissingParams(t *testing.T) {
	h := withReports(&fakeReports{})
	for _, path := range []string{
		"/api/v1/reports/ecad",
		"/api/v1/reports/ecad?from=2026-07-24T00:00:00Z",
		"/api/v1/reports/ecad?from=bad&to=2026-07-25T00:00:00Z",
	} {
		if rec := do(t, h, http.MethodGet, path, ""); rec.Code != 400 {
			t.Fatalf("%s should 400, got %d", path, rec.Code)
		}
	}
}

func TestGetECADReportBadRange(t *testing.T) {
	h := withReports(&fakeReports{err: inbound.ErrBadRange})
	rec := do(t, h, http.MethodGet,
		"/api/v1/reports/ecad?from=2026-07-25T00:00:00Z&to=2026-07-24T00:00:00Z", "")
	if rec.Code != 422 {
		t.Fatalf("bad range should 422, got %d", rec.Code)
	}
}

func TestGetECADReportRepoError(t *testing.T) {
	h := withReports(&fakeReports{err: context.DeadlineExceeded})
	rec := do(t, h, http.MethodGet,
		"/api/v1/reports/ecad?from=2026-07-24T00:00:00Z&to=2026-07-25T00:00:00Z", "")
	if rec.Code != 500 {
		t.Fatalf("repo error should 500, got %d", rec.Code)
	}
}

func TestReportsRouteAbsentWithoutOption(t *testing.T) {
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil)
	if rec := do(t, h, http.MethodGet, "/api/v1/reports/ecad?from=x&to=y", ""); rec.Code != 404 {
		t.Fatalf("should 404 without WithReports, got %d", rec.Code)
	}
	if rec := do(t, h, http.MethodGet, "/api/v1/reports/broadcast?from=x&to=y", ""); rec.Code != 404 {
		t.Fatalf("broadcast should 404 without WithReports, got %d", rec.Code)
	}
}

func TestGetBroadcastProofOK(t *testing.T) {
	from := time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)
	to := from.Add(24 * time.Hour)
	at := from.Add(90 * time.Minute)
	rp := &fakeReports{proof: reports.BroadcastProof{
		From: from, To: to, GeneratedAt: to.Add(time.Hour),
		Lines: []reports.ProofLine{{
			EntryID: "e1", PlayedAt: at, TrackID: "a", Title: "Aurora",
			Artist: "Nébula", Source: shared.SourceLocal, DurationMs: 180000,
		}},
		TotalEntries:    1,
		TotalDurationMs: 180000,
	}}
	h := withReports(rp)
	rec := do(t, h, http.MethodGet,
		"/api/v1/reports/broadcast?from=2026-07-24T00:00:00Z&to=2026-07-25T00:00:00Z", "")
	if rec.Code != 200 {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body)
	}
	if !rp.gotFrom.Equal(from) || !rp.gotTo.Equal(to) {
		t.Fatalf("range parsed wrong: from=%v to=%v", rp.gotFrom, rp.gotTo)
	}
	var got broadcastProofDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.TotalEntries != 1 || len(got.Lines) != 1 || got.Lines[0].EntryID != "e1" ||
		got.Lines[0].Source != "local" || got.Lines[0].DurationMs != 180000 {
		t.Fatalf("dto=%+v", got)
	}
}

func TestGetBroadcastProofMissingParams(t *testing.T) {
	h := withReports(&fakeReports{})
	for _, path := range []string{
		"/api/v1/reports/broadcast",
		"/api/v1/reports/broadcast?from=2026-07-24T00:00:00Z",
		"/api/v1/reports/broadcast?from=bad&to=2026-07-25T00:00:00Z",
	} {
		if rec := do(t, h, http.MethodGet, path, ""); rec.Code != 400 {
			t.Fatalf("%s should 400, got %d", path, rec.Code)
		}
	}
}

func TestGetBroadcastProofBadRange(t *testing.T) {
	h := withReports(&fakeReports{err: inbound.ErrBadRange})
	rec := do(t, h, http.MethodGet,
		"/api/v1/reports/broadcast?from=2026-07-25T00:00:00Z&to=2026-07-24T00:00:00Z", "")
	if rec.Code != 422 {
		t.Fatalf("bad range should 422, got %d", rec.Code)
	}
}

func TestGetBroadcastProofRepoError(t *testing.T) {
	h := withReports(&fakeReports{err: context.DeadlineExceeded})
	rec := do(t, h, http.MethodGet,
		"/api/v1/reports/broadcast?from=2026-07-24T00:00:00Z&to=2026-07-25T00:00:00Z", "")
	if rec.Code != 500 {
		t.Fatalf("repo error should 500, got %d", rec.Code)
	}
}
