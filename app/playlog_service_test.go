package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"autodj/domain/library"
	"autodj/domain/playlog"
	"autodj/domain/shared"
	"autodj/ports/inbound"
)

// --- fake PlayLogRepo ---

type memPlayLogRepo struct {
	entries []playlog.Entry
	failOn  string // "Append" | "Since" | "Top" → errPlBoom
}

var errPlBoom = errors.New("boom")

func (r *memPlayLogRepo) Append(_ context.Context, e playlog.Entry) error {
	if r.failOn == "Append" {
		return errPlBoom
	}
	for _, x := range r.entries { // idempotent on ID
		if x.ID == e.ID {
			return nil
		}
	}
	r.entries = append(r.entries, e)
	return nil
}

func (r *memPlayLogRepo) Since(_ context.Context, from, to time.Time) ([]playlog.Entry, error) {
	if r.failOn == "Since" {
		return nil, errPlBoom
	}
	var out []playlog.Entry
	for _, e := range r.entries {
		if !e.PlayedAt.Before(from) && e.PlayedAt.Before(to) {
			out = append(out, e)
		}
	}
	return out, nil
}

func (r *memPlayLogRepo) Top(_ context.Context, from, to time.Time, n int) ([]playlog.TopItem, error) {
	if r.failOn == "Top" {
		return nil, errPlBoom
	}
	var window []playlog.Entry
	for _, e := range r.entries {
		if !e.PlayedAt.Before(from) && e.PlayedAt.Before(to) {
			window = append(window, e)
		}
	}
	return playlog.TopN(window, n), nil
}

// fixedClock is declared in content_service_test.go (same package).

func trk(id string, src shared.SourceKind) library.Track {
	return library.Track{ID: id, Title: "T-" + id, Artist: "A-" + id, Source: src, Duration: time.Minute}
}

// --- tests ---

func TestPlayLogRecord(t *testing.T) {
	repo := &memPlayLogRepo{}
	svc := NewPlayLogService(repo, fixedClock{time.Now()})
	at := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	if err := svc.Record(context.Background(), trk("s1", shared.SourceLocal), at); err != nil {
		t.Fatalf("record: %v", err)
	}
	if len(repo.entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(repo.entries))
	}
	e := repo.entries[0]
	if !e.Broadcasted || !e.ECADRelevant {
		t.Errorf("local entry not broadcast/ECAD: %+v", e)
	}
	// idempotent: same track+instant does not double-count
	_ = svc.Record(context.Background(), trk("s1", shared.SourceLocal), at)
	if len(repo.entries) != 1 {
		t.Errorf("re-record changed count to %d", len(repo.entries))
	}
}

func TestPlayLogRecordBadEntry(t *testing.T) {
	svc := NewPlayLogService(&memPlayLogRepo{}, fixedClock{time.Now()})
	err := svc.Record(context.Background(), trk("", shared.SourceLocal), time.Now())
	if !errors.Is(err, playlog.ErrBadEntry) {
		t.Fatalf("err = %v, want ErrBadEntry", err)
	}
}

func TestPlayLogRecordRepoError(t *testing.T) {
	repo := &memPlayLogRepo{failOn: "Append"}
	svc := NewPlayLogService(repo, fixedClock{time.Now()})
	err := svc.Record(context.Background(), trk("s1", shared.SourceLocal), time.Now())
	if !errors.Is(err, errPlBoom) {
		t.Fatalf("err = %v, want boom", err)
	}
}

func TestPlayLogHistory(t *testing.T) {
	repo := &memPlayLogRepo{}
	svc := NewPlayLogService(repo, fixedClock{time.Now()})
	base := time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)
	_ = svc.Record(context.Background(), trk("a", shared.SourceLocal), base)
	_ = svc.Record(context.Background(), trk("b", shared.SourceLocal), base.Add(2*time.Hour))
	got, err := svc.History(context.Background(), base, base.Add(time.Hour))
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(got) != 1 || got[0].TrackID != "a" {
		t.Fatalf("history window mismatch: %+v", got)
	}
}

func TestPlayLogHistoryRepoError(t *testing.T) {
	repo := &memPlayLogRepo{failOn: "Since"}
	svc := NewPlayLogService(repo, fixedClock{time.Now()})
	if _, err := svc.History(context.Background(), time.Time{}, time.Now()); !errors.Is(err, errPlBoom) {
		t.Fatalf("err = %v, want boom", err)
	}
}

func TestPlayLogTopPeriods(t *testing.T) {
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	repo := &memPlayLogRepo{}
	svc := NewPlayLogService(repo, fixedClock{now})
	// two plays of "hot" 20h ago (inside dia), one of "cold" 5 days ago (outside dia, inside semana)
	svc.repoRecord(trk("hot", shared.SourceLocal), now.Add(-20*time.Hour), 0)
	svc.repoRecord(trk("hot", shared.SourceLocal), now.Add(-20*time.Hour), 1)
	svc.repoRecord(trk("cold", shared.SourceLocal), now.Add(-5*24*time.Hour), 0)

	dia, err := svc.Top(context.Background(), "dia", 10)
	if err != nil {
		t.Fatalf("top dia: %v", err)
	}
	if len(dia) != 1 || dia[0].TrackID != "hot" || dia[0].Count != 2 {
		t.Fatalf("dia = %+v, want hot/2", dia)
	}
	semana, _ := svc.Top(context.Background(), "semana", 10)
	if len(semana) != 2 {
		t.Fatalf("semana = %+v, want 2 tracks", semana)
	}
	mes, _ := svc.Top(context.Background(), "mes", 10)
	if len(mes) != 2 {
		t.Fatalf("mes = %+v, want 2 tracks", mes)
	}
}

func TestPlayLogTopBadPeriod(t *testing.T) {
	svc := NewPlayLogService(&memPlayLogRepo{}, fixedClock{time.Now()})
	if _, err := svc.Top(context.Background(), "ano", 5); !errors.Is(err, inbound.ErrBadPeriod) {
		t.Fatalf("err = %v, want ErrBadPeriod", err)
	}
}

func TestPlayLogTopRepoError(t *testing.T) {
	repo := &memPlayLogRepo{failOn: "Top"}
	svc := NewPlayLogService(repo, fixedClock{time.Now()})
	if _, err := svc.Top(context.Background(), "dia", 5); !errors.Is(err, errPlBoom) {
		t.Fatalf("err = %v, want boom", err)
	}
}

// repoRecord builds a distinct-ID entry directly (bypassing NewEntry's
// instant-based ID collision) so a fixture can log the same track+instant twice.
func (s *PlayLogService) repoRecord(t library.Track, at time.Time, seq int) {
	e, _ := playlog.NewEntry(t, at)
	e.ID = e.ID + "#" + time.Duration(seq).String()
	_ = s.repo.Append(context.Background(), e)
}
