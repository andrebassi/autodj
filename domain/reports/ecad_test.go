package reports_test

import (
	"testing"
	"time"

	"autodj/domain/playlog"
	"autodj/domain/reports"
	"autodj/domain/shared"
)

// base window [from, to): a full day.
var (
	from = time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)
	to   = time.Date(2026, 7, 25, 0, 0, 0, 0, time.UTC)
	gen  = time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
)

// entry is a test helper that builds an Entry directly (bypassing NewEntry) so a
// test can set ECADRelevant/PlayedAt precisely without a Track fixture.
func entry(id, title string, at time.Time, ms int64, ecad bool) playlog.Entry {
	return playlog.Entry{
		ID:           id,
		TrackID:      id,
		Title:        title,
		Artist:       "Nébula",
		Source:       shared.SourceLocal,
		PlayedAt:     at,
		DurationMs:   ms,
		Broadcasted:  ecad,
		ECADRelevant: ecad,
	}
}

func TestBuildECAD_AggregatesSameTrack(t *testing.T) {
	at1 := from.Add(1 * time.Hour)
	at2 := from.Add(2 * time.Hour)
	rep := reports.BuildECAD([]playlog.Entry{
		entry("a", "Aurora", at1, 180000, true),
		entry("a", "Aurora", at2, 180000, true),
	}, from, to, gen)

	if len(rep.Lines) != 1 {
		t.Fatalf("want 1 line, got %d", len(rep.Lines))
	}
	l := rep.Lines[0]
	if l.Plays != 2 || l.TotalDurationMs != 360000 {
		t.Fatalf("want plays=2 dur=360000, got plays=%d dur=%d", l.Plays, l.TotalDurationMs)
	}
	if rep.TotalPlays != 2 || rep.TotalDurationMs != 360000 {
		t.Fatalf("totals wrong: plays=%d dur=%d", rep.TotalPlays, rep.TotalDurationMs)
	}
	if !rep.From.Equal(from) || !rep.To.Equal(to) || !rep.GeneratedAt.Equal(gen) {
		t.Fatalf("window/generatedAt not preserved")
	}
}

func TestBuildECAD_IgnoresOutOfWindow(t *testing.T) {
	before := from.Add(-1 * time.Hour)
	atTo := to // exactly To is excluded ([from, to))
	rep := reports.BuildECAD([]playlog.Entry{
		entry("a", "Aurora", before, 180000, true),
		entry("b", "Bloom", atTo, 180000, true),
		entry("c", "Cume", from, 120000, true), // exactly from is included
	}, from, to, gen)

	if len(rep.Lines) != 1 || rep.Lines[0].TrackID != "c" {
		t.Fatalf("want only track c in window, got %+v", rep.Lines)
	}
	if rep.TotalPlays != 1 {
		t.Fatalf("want total 1, got %d", rep.TotalPlays)
	}
}

func TestBuildECAD_IgnoresNonECAD(t *testing.T) {
	at := from.Add(1 * time.Hour)
	rep := reports.BuildECAD([]playlog.Entry{
		entry("spot", "DRM Tune", at, 200000, false), // Spotify: ECADRelevant=false
		entry("loc", "Local Tune", at, 180000, true),
	}, from, to, gen)

	if len(rep.Lines) != 1 || rep.Lines[0].TrackID != "loc" {
		t.Fatalf("non-ECAD entry leaked: %+v", rep.Lines)
	}
	if rep.TotalPlays != 1 || rep.TotalDurationMs != 180000 {
		t.Fatalf("non-ECAD counted in totals: plays=%d dur=%d", rep.TotalPlays, rep.TotalDurationMs)
	}
}

func TestBuildECAD_OrdersByPlaysThenTitle(t *testing.T) {
	at := from.Add(1 * time.Hour)
	rep := reports.BuildECAD([]playlog.Entry{
		entry("z", "Zephyr", at, 100000, true),  // 1 play
		entry("b", "Bloom", at, 100000, true),   // 2 plays
		entry("b", "Bloom", at, 100000, true),   //
		entry("a", "Aurora", at, 100000, true),  // 2 plays, tie with Bloom → Aurora first (Title)
		entry("a", "Aurora", at, 100000, true),  //
	}, from, to, gen)

	got := []string{rep.Lines[0].Title, rep.Lines[1].Title, rep.Lines[2].Title}
	want := []string{"Aurora", "Bloom", "Zephyr"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order wrong: got %v want %v", got, want)
		}
	}
}

func TestBuildECAD_EmptyYieldsZeroTotals(t *testing.T) {
	rep := reports.BuildECAD(nil, from, to, gen)
	if len(rep.Lines) != 0 || rep.TotalPlays != 0 || rep.TotalDurationMs != 0 {
		t.Fatalf("want empty report, got %+v", rep)
	}
	if !rep.GeneratedAt.Equal(gen) {
		t.Fatalf("generatedAt lost on empty report")
	}
}
