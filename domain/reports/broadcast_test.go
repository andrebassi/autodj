package reports_test

import (
	"testing"
	"time"

	"autodj/domain/playlog"
	"autodj/domain/reports"
)

// reuses entry() and the from/to/gen window vars declared in ecad_test.go
// (same reports_test package). entry() sets Broadcasted == the bool arg, so
// entry(..., true) is an aired play and entry(..., false) is a non-broadcast one.

func TestBuildBroadcastProof_OneLinePerPlayChronological(t *testing.T) {
	at1 := from.Add(2 * time.Hour)
	at2 := from.Add(1 * time.Hour) // earlier — must sort first
	proof := reports.BuildBroadcastProof([]playlog.Entry{
		entry("a2", "Aurora", at1, 180000, true),
		entry("a1", "Aurora", at2, 180000, true), // same track, 2 plays → 2 lines
	}, from, to, gen)

	if len(proof.Lines) != 2 {
		t.Fatalf("want 2 lines (no aggregation), got %d", len(proof.Lines))
	}
	if !proof.Lines[0].PlayedAt.Equal(at2) || !proof.Lines[1].PlayedAt.Equal(at1) {
		t.Fatalf("not chronological: %v then %v", proof.Lines[0].PlayedAt, proof.Lines[1].PlayedAt)
	}
	if proof.TotalEntries != 2 || proof.TotalDurationMs != 360000 {
		t.Fatalf("totals wrong: n=%d dur=%d", proof.TotalEntries, proof.TotalDurationMs)
	}
	if !proof.From.Equal(from) || !proof.To.Equal(to) || !proof.GeneratedAt.Equal(gen) {
		t.Fatalf("window/generatedAt not preserved")
	}
}

func TestBuildBroadcastProof_IgnoresOutOfWindow(t *testing.T) {
	before := from.Add(-1 * time.Hour)
	atTo := to // exactly To is excluded ([from, to))
	proof := reports.BuildBroadcastProof([]playlog.Entry{
		entry("a", "Aurora", before, 180000, true),
		entry("b", "Bloom", atTo, 180000, true),
		entry("c", "Cume", from, 120000, true), // exactly from is included
	}, from, to, gen)

	if len(proof.Lines) != 1 || proof.Lines[0].TrackID != "c" {
		t.Fatalf("want only track c in window, got %+v", proof.Lines)
	}
	if proof.TotalEntries != 1 || proof.TotalDurationMs != 120000 {
		t.Fatalf("totals wrong: n=%d dur=%d", proof.TotalEntries, proof.TotalDurationMs)
	}
}

func TestBuildBroadcastProof_IgnoresNonBroadcast(t *testing.T) {
	at := from.Add(1 * time.Hour)
	proof := reports.BuildBroadcastProof([]playlog.Entry{
		entry("spot", "DRM Tune", at, 200000, false), // Spotify: Broadcasted=false
		entry("loc", "Local Tune", at, 180000, true),
	}, from, to, gen)

	if len(proof.Lines) != 1 || proof.Lines[0].TrackID != "loc" {
		t.Fatalf("non-broadcast entry leaked: %+v", proof.Lines)
	}
	if proof.TotalEntries != 1 || proof.TotalDurationMs != 180000 {
		t.Fatalf("non-broadcast counted in totals: n=%d dur=%d", proof.TotalEntries, proof.TotalDurationMs)
	}
}

func TestBuildBroadcastProof_TieBreakByEntryID(t *testing.T) {
	at := from.Add(1 * time.Hour) // identical timestamps → tie-break by EntryID
	proof := reports.BuildBroadcastProof([]playlog.Entry{
		entry("z", "Zephyr", at, 100000, true),
		entry("a", "Aurora", at, 100000, true),
	}, from, to, gen)

	if proof.Lines[0].EntryID != "a" || proof.Lines[1].EntryID != "z" {
		t.Fatalf("tie not broken by EntryID: %s then %s", proof.Lines[0].EntryID, proof.Lines[1].EntryID)
	}
}

func TestBuildBroadcastProof_EmptyYieldsZeroTotals(t *testing.T) {
	proof := reports.BuildBroadcastProof(nil, from, to, gen)
	if len(proof.Lines) != 0 || proof.TotalEntries != 0 || proof.TotalDurationMs != 0 {
		t.Fatalf("want empty proof, got %+v", proof)
	}
	if !proof.GeneratedAt.Equal(gen) {
		t.Fatalf("generatedAt lost on empty proof")
	}
}
