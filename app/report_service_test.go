package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"autodj/domain/shared"
	"autodj/ports/inbound"
)

func TestReportECAD_AggregatesWindow(t *testing.T) {
	from := time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)
	to := from.Add(24 * time.Hour)
	gen := to.Add(6 * time.Hour)
	repo := &memPlayLogRepo{}
	svc := NewReportService(repo, fixedClock{gen})

	// two local plays of "a" in window, one Spotify play (excluded from ECAD).
	svc2 := NewPlayLogService(repo, fixedClock{gen})
	svc2.repoRecord(trk("a", shared.SourceLocal), from.Add(1*time.Hour), 0)
	svc2.repoRecord(trk("a", shared.SourceLocal), from.Add(2*time.Hour), 1)
	svc2.repoRecord(trk("z", shared.SourceSpotify), from.Add(3*time.Hour), 0)

	rep, err := svc.ECAD(context.Background(), from, to)
	if err != nil {
		t.Fatalf("ecad: %v", err)
	}
	if len(rep.Lines) != 1 || rep.Lines[0].TrackID != "a" || rep.Lines[0].Plays != 2 {
		t.Fatalf("lines = %+v, want single track a with 2 plays", rep.Lines)
	}
	if rep.TotalPlays != 2 {
		t.Fatalf("total plays = %d, want 2 (Spotify excluded)", rep.TotalPlays)
	}
	if !rep.GeneratedAt.Equal(gen) {
		t.Fatalf("generatedAt = %v, want clock %v", rep.GeneratedAt, gen)
	}
}

func TestReportECAD_BadRange(t *testing.T) {
	svc := NewReportService(&memPlayLogRepo{}, fixedClock{time.Now()})
	now := time.Now()
	if _, err := svc.ECAD(context.Background(), now, now); !errors.Is(err, inbound.ErrBadRange) {
		t.Fatalf("equal range err = %v, want ErrBadRange", err)
	}
	if _, err := svc.ECAD(context.Background(), now.Add(time.Hour), now); !errors.Is(err, inbound.ErrBadRange) {
		t.Fatalf("inverted range err = %v, want ErrBadRange", err)
	}
}

func TestReportECAD_RepoError(t *testing.T) {
	repo := &memPlayLogRepo{failOn: "Since"}
	svc := NewReportService(repo, fixedClock{time.Now()})
	from := time.Now()
	if _, err := svc.ECAD(context.Background(), from, from.Add(time.Hour)); !errors.Is(err, errPlBoom) {
		t.Fatalf("err = %v, want boom", err)
	}
}

func TestReportBroadcastProof_ChronologicalWindow(t *testing.T) {
	from := time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)
	to := from.Add(24 * time.Hour)
	gen := to.Add(6 * time.Hour)
	repo := &memPlayLogRepo{}
	svc := NewReportService(repo, fixedClock{gen})

	// two local aired plays of "a" + one Spotify play (Broadcasted=false → excluded).
	svc2 := NewPlayLogService(repo, fixedClock{gen})
	svc2.repoRecord(trk("a", shared.SourceLocal), from.Add(2*time.Hour), 0)
	svc2.repoRecord(trk("a", shared.SourceLocal), from.Add(1*time.Hour), 1)
	svc2.repoRecord(trk("z", shared.SourceSpotify), from.Add(3*time.Hour), 0)

	proof, err := svc.BroadcastProof(context.Background(), from, to)
	if err != nil {
		t.Fatalf("broadcast proof: %v", err)
	}
	if proof.TotalEntries != 2 {
		t.Fatalf("total entries = %d, want 2 (Spotify excluded, no aggregation)", proof.TotalEntries)
	}
	// chronological: the earlier play (from+1h) must come first.
	if !proof.Lines[0].PlayedAt.Equal(from.Add(1 * time.Hour)) {
		t.Fatalf("not chronological: first = %v", proof.Lines[0].PlayedAt)
	}
	for _, l := range proof.Lines {
		if l.Source == shared.SourceSpotify {
			t.Fatalf("Spotify (non-broadcast) leaked into proof: %+v", l)
		}
	}
	if !proof.GeneratedAt.Equal(gen) {
		t.Fatalf("generatedAt = %v, want clock %v", proof.GeneratedAt, gen)
	}
}

func TestReportBroadcastProof_BadRange(t *testing.T) {
	svc := NewReportService(&memPlayLogRepo{}, fixedClock{time.Now()})
	now := time.Now()
	if _, err := svc.BroadcastProof(context.Background(), now, now); !errors.Is(err, inbound.ErrBadRange) {
		t.Fatalf("equal range err = %v, want ErrBadRange", err)
	}
	if _, err := svc.BroadcastProof(context.Background(), now.Add(time.Hour), now); !errors.Is(err, inbound.ErrBadRange) {
		t.Fatalf("inverted range err = %v, want ErrBadRange", err)
	}
}

func TestReportBroadcastProof_RepoError(t *testing.T) {
	repo := &memPlayLogRepo{failOn: "Since"}
	svc := NewReportService(repo, fixedClock{time.Now()})
	from := time.Now()
	if _, err := svc.BroadcastProof(context.Background(), from, from.Add(time.Hour)); !errors.Is(err, errPlBoom) {
		t.Fatalf("err = %v, want boom", err)
	}
}
